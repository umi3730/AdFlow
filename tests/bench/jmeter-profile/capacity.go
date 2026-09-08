package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func newCapacityHTTPClient() *http.Client { return &http.Client{Timeout: 5 * time.Second} }

func runCapacity(output string, probeSeconds, confirmSeconds int) (err error) {
	if probeSeconds < 5 || probeSeconds > 60 || confirmSeconds < 30 || confirmSeconds > 120 {
		return fmt.Errorf("probe 5..60s and confirmation 30..120s required")
	}
	dir, e := filepath.Abs(output)
	if e != nil {
		return e
	}
	if e = os.Mkdir(dir, 0755); e != nil {
		return e
	}
	h := &lab{dir: dir, schema: "adflow_qps_" + time.Now().UTC().Format("20060102_150405"), client: newCapacityHTTPClient(), report: map[string]any{"startedAt": time.Now().UTC(), "endpoint": "GET /v1/profiles/{userId}", "adapter": "mysql-redis", "users": 1000, "probeSeconds": probeSeconds, "confirmSeconds": confirmSeconds, "slo": map[string]any{"p95MsLessThan": 100, "errorRateLessThan": 0.001, "droppedIterations": 0, "minimumDispatchFraction": 0.99}, "searchResolutionQPS": 500, "maxProbeQPS": 16000, "notes": []string{"Same-host k6/API/Redis/MySQL, prewarmed 1000-row working set, JWT enabled. Single endpoint only; no decision gate, budget or Kafka workload.", "No code, logging, pool, timeout or cache optimization during search. Original API logs retained on the workspace volume.", "Each probe clears only its dedicated Redis instance then warms all 1000 profiles, so cache expiry does not mix cold/hot phases.", "Maximum means highest locally verified offered rate under stated SLO and harness limits; shared-host load generator can limit observed capacity.", "CPU percent is relative to one logical CPU (100% = one core); host CPU percent is normalized across all logical CPUs. /proc counters sampled every second; GET /metrics is extra observer traffic.", "Probe SLO failures are data, not hidden/retried within a run. Three longer confirmations are required for a stable claim."}}}
	defer func() {
		if err != nil {
			h.report["error"] = err.Error()
		}
		cleanup := map[string]any{"apiStopped": stop(h.api) == nil, "redisStopped": stop(h.rp) == nil}
		if h.rc != nil {
			_ = h.rc.Close()
		}
		if h.db != nil {
			_ = h.db.Close()
		}
		if h.admin != nil {
			if h.created {
				_, e := h.admin.Exec("DROP DATABASE `" + h.schema + "`")
				cleanup["schemaDropped"] = e == nil
			}
			_ = h.admin.Close()
		}
		h.report["cleanup"] = cleanup
		h.report["finishedAt"] = time.Now().UTC()
		_ = h.save()
	}()
	if err = h.setup(); err != nil {
		return
	}
	if err = h.start("mysql-redis", "capacity"); err != nil {
		return
	}
	hashes := h.report["sourceSHA256"].(map[string]string)
	for _, name := range []string{"tests/bench/jmeter-profile/capacity.go", "tests/load/profile-capacity.js"} {
		b, e := os.ReadFile(name)
		if e != nil {
			return e
		}
		hashes[name] = fmt.Sprintf("%x", sha256.Sum256(b))
	}
	v, e := exec.Command("work/k6-lab/k6-v2.2.0-linux-amd64/k6", "version").CombinedOutput()
	if e != nil {
		return e
	}
	h.report["k6Version"] = strings.TrimSpace(string(v))
	h.report["logicalCPUs"] = runtime.NumCPU()
	lo, hi := 0, 0
	run := func(rate, seconds int, label string) (bool, error) {
		r, e := h.capacityProbe(rate, seconds, label)
		h.results = append(h.results, r)
		h.report["results"] = h.results
		_ = h.save()
		if e != nil {
			return false, e
		}
		return r["passed"].(bool), nil
	}
	for _, rate := range []int{500, 1000, 2000, 4000, 8000, 16000} {
		ok, e := run(rate, probeSeconds, "ramp")
		if e != nil {
			return e
		}
		if !ok {
			hi = rate
			break
		}
		lo = rate
	}
	if lo == 0 {
		h.report["conclusion"] = "No initial rate passed; inspect resource data and failures"
		return nil
	}
	for hi > lo+500 {
		mid := ((lo + hi) / 2 / 500) * 500
		if mid <= lo {
			break
		}
		ok, e := run(mid, probeSeconds, "refine")
		if e != nil {
			return e
		}
		if ok {
			lo = mid
		} else {
			hi = mid
		}
	}
	h.report["shortProbeBracket"] = map[string]int{"passedQPS": lo, "failedQPS": hi}
	for attempt := 0; attempt < 4 && lo > 0; attempt++ {
		all := true
		for repeat := 0; repeat < 3; repeat++ {
			ok, e := run(lo, confirmSeconds, fmt.Sprintf("confirm-%d", repeat+1))
			if e != nil {
				return e
			}
			if !ok {
				all = false
				break
			}
		}
		if all {
			h.report["confirmedOfferedQPS"] = lo
			h.report["confirmationsPassed"] = 3
			h.report["ceilingReachedWithoutFailure"] = hi == 0
			return nil
		}
		hi = lo
		lo -= 500
	}
	h.report["conclusion"] = "No rate achieved three long confirmations within bounded search; do not claim a stable maximum"
	return nil
}

type procReading struct {
	Ticks   int64
	RSS     int64
	Threads int64
}

func readProc(pid int) procReading {
	raw, e := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if e != nil {
		return procReading{}
	}
	text := string(raw)
	i := strings.LastIndex(text, ")")
	fields := strings.Fields(text[i+1:])
	if len(fields) < 22 {
		return procReading{}
	}
	number := func(i int) int64 { v, _ := strconv.ParseInt(fields[i], 10, 64); return v }
	return procReading{Ticks: number(11) + number(12), RSS: number(21) * int64(os.Getpagesize()), Threads: number(17)}
}
func readHost() (int64, int64) {
	raw, e := os.ReadFile("/proc/stat")
	if e != nil {
		return 0, 0
	}
	fields := strings.Fields(strings.SplitN(string(raw), "\n", 2)[0])
	var total, idle int64
	for i := 1; i < len(fields) && i <= 8; i++ {
		v, _ := strconv.ParseInt(fields[i], 10, 64)
		total += v
		if i == 4 || i == 5 {
			idle += v
		}
	}
	return total, idle
}
func (h *lab) capacityProbe(rate, seconds int, label string) (r map[string]any, err error) {
	id := fmt.Sprintf("%02d-%s-%d", len(h.results)+1, label, rate)
	r = map[string]any{"id": id, "label": label, "offeredQPS": rate, "seconds": seconds}
	fmt.Printf("START %s for %ds\n", id, seconds)
	// This client belongs to the standalone Redis process created by setup.
	if err = h.rc.FlushDB(context.Background()).Err(); err != nil {
		return
	}
	for i := 0; i < 1000; i++ {
		if _, err = h.request("GET", fmt.Sprintf("/v1/profiles/profile-%04d", i), nil); err != nil {
			return
		}
	}
	before, e := h.tableFetches()
	if e != nil {
		err = e
		return
	}
	cacheBefore, e := h.cacheCounters()
	if e != nil {
		err = e
		return
	}
	rawPath := filepath.Join(h.dir, id+"-k6.json")
	args := []string{"run", "--no-usage-report", "--no-color", "--quiet", "-e", "BASE_URL=" + h.base, "-e", fmt.Sprintf("RATE=%d", rate), "-e", fmt.Sprintf("SECONDS=%d", seconds), "-e", "REPORT_PATH=" + rawPath, "tests/load/profile-capacity.js"}
	cmd := exec.Command("work/k6-lab/k6-v2.2.0-linux-amd64/k6", args...)
	cmd.Env = append(os.Environ(), "ADFLOW_CAPACITY_TOKEN="+h.token)
	if err = launch(cmd, filepath.Join(h.dir, id+"-console.log")); err != nil {
		return
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	pids := map[string]int{"api": h.api.Process.Pid, "injector": cmd.Process.Pid, "redis": h.rp.Process.Pid}
	if raw, e := exec.Command("pidof", "mysqld").Output(); e == nil {
		v, _ := strconv.Atoi(strings.Fields(string(raw))[0])
		pids["mysql"] = v
	}
	clockRaw, e := exec.Command("getconf", "CLK_TCK").Output()
	if e != nil {
		err = e
		return
	}
	clock, _ := strconv.ParseFloat(strings.TrimSpace(string(clockRaw)), 64)
	if clock <= 0 {
		err = fmt.Errorf("invalid CPU clock")
		return
	}
	previous := map[string]procReading{}
	for name, pid := range pids {
		previous[name] = readProc(pid)
	}
	hostTotal, hostIdle := readHost()
	last := time.Now()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var resources []map[string]any
	var runErr error
	running := true
	for running {
		select {
		case runErr = <-done:
			running = false
		case <-ticker.C:
			now := time.Now()
			elapsed := now.Sub(last).Seconds()
			point := map[string]any{"at": now.UTC()}
			for name, pid := range pids {
				v := readProc(pid)
				old := previous[name]
				if v.Ticks >= old.Ticks && v.RSS > 0 {
					point[name] = map[string]any{"cpuPercentOneCore": float64(v.Ticks-old.Ticks) / clock / elapsed * 100, "rssBytes": v.RSS, "osThreads": v.Threads}
				}
				previous[name] = v
			}
			total, idle := readHost()
			if total > hostTotal {
				point["hostBusyPercent"] = 100 * (1 - float64(idle-hostIdle)/float64(total-hostTotal))
			}
			hostTotal, hostIdle = total, idle
			if raw, e := h.request("GET", "/metrics", nil); e == nil {
				for _, line := range strings.Split(string(raw), "\n") {
					for _, metric := range []string{"go_goroutines", "go_sched_gomaxprocs_threads"} {
						if strings.HasPrefix(line, metric+" ") {
							v, _ := strconv.ParseFloat(strings.TrimPrefix(line, metric+" "), 64)
							point[metric] = v
						}
					}
				}
			}
			resources = append(resources, point)
			last = now
		}
	}
	if runErr != nil {
		r["k6Exit"] = runErr.Error()
	}
	raw, e := os.ReadFile(rawPath)
	if e != nil {
		err = e
		return
	}
	var summary struct {
		Metrics map[string]struct {
			Values map[string]float64 `json:"values"`
		} `json:"metrics"`
	}
	if e = json.Unmarshal(raw, &summary); e != nil {
		err = e
		return
	}
	values := func(name string) map[string]float64 { return summary.Metrics[name].Values }
	requests := values("http_reqs")["count"]
	valid := values("valid_profiles")["count"]
	p95 := values("http_req_duration")["p(95)"]
	errorRate := values("profile_errors")["rate"]
	dropped := values("dropped_iterations")["count"]
	after, e := h.tableFetches()
	if e != nil {
		err = e
		return
	}
	cacheAfter, e := h.cacheCounters()
	if e != nil {
		err = e
		return
	}
	for k, v := range cacheBefore {
		cacheAfter[k] -= v
	}
	r["httpMetrics"] = values("http_req_duration")
	r["requests"] = requests
	r["validResponses"] = valid
	r["observedHTTPQPS"] = values("http_reqs")["rate"]
	r["errors"] = errorRate
	r["droppedIterations"] = dropped
	r["resources"] = resources
	r["tableFetches"] = after - before
	r["cacheCounters"] = cacheAfter
	r["passed"] = runErr == nil && p95 < 100 && errorRate < 0.001 && dropped == 0 && requests >= float64(rate*seconds)*0.99 && valid >= requests*0.999
	fmt.Printf("END %s: HTTP %.1f/s, p95 %.2fms, error %.4f, dropped %.0f, pass=%v\n", id, values("http_reqs")["rate"], p95, errorRate, dropped, r["passed"])
	return
}
