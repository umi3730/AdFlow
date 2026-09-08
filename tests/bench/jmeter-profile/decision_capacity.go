package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/redis/go-redis/v9"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (h *lab) createForDecision(path string, body []byte, expected int) ([]byte, error) {
	req, e := http.NewRequest("POST", h.base+path, bytes.NewReader(body))
	if e != nil {
		return nil, e
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+h.token)
	res, e := h.client.Do(req)
	if e != nil {
		return nil, e
	}
	defer res.Body.Close()
	raw, e := io.ReadAll(res.Body)
	if e != nil {
		return nil, e
	}
	if res.StatusCode != expected {
		return nil, fmt.Errorf("create fixture %s status %d", path, res.StatusCode)
	}
	return raw, nil
}

const decisionBudgetFen int64 = 10000000

func runDecisionCapacity(output string, probeSeconds, confirmSeconds int) (err error) {
	if probeSeconds < 5 || probeSeconds > 60 || confirmSeconds < 30 || confirmSeconds > 120 {
		return fmt.Errorf("invalid bounded durations")
	}
	dir, e := filepath.Abs(output)
	if e != nil {
		return e
	}
	if e = os.Mkdir(dir, 0755); e != nil {
		return e
	}
	h := &lab{dir: dir, schema: "adflow_decqps_" + time.Now().UTC().Format("20060102_150405"), client: newCapacityHTTPClient(), apiOverrides: map[string]string{"DECISION_RATE_LIMITER": "redis"}, report: map[string]any{
		"startedAt": time.Now().UTC(), "endpoint": "POST /v1/decisions", "probeSeconds": probeSeconds, "confirmSeconds": confirmSeconds, "users": 1000, "advertisers": 3, "bidFen": []int{2, 3, 5}, "budgetFen": decisionBudgetFen, "frequencyLimit": 100,
		"slo": map[string]any{"p95MsLessThan": 300, "decisionErrorRateLessThan": 0.001, "droppedIterations": 0, "noAd": 0}, "resolutionQPS": 25, "probeCeilingQPS": 2000,
		"notes": []string{"Decision-only: no impression/click/conversion calls, no Kafka work. Includes auth, warm persisted profile lookup, targeting/auction, real Redis rate limiting/frequency/budget reservation, MySQL execution lease and decision commit, normal audit/logging.",
			"Three same-slot advertisers; every user qualifies. New request ID per iteration; no cached decision replay contributes to QPS. Client validates winner, price, advertiser count and identity.",
			"Each stage clears only this runner's dedicated Redis instance, then warms 1000 profiles and 3 decisions. Never reset counters during measured load. MySQL decisions from prior stages remain under unique prefixes.",
			"No exposures: Redis spent must remain zero. Budget/frequency gates stay enabled. At ceiling 2000/s, a 30s reservation window averages 60 reservations per user, below frequency limit 100; budget comfortably exceeds worst-case reservations.",
			"20s short probes, 60s confirmations by default; 25-QPS resolution, three passing confirmation runs required. Preallocated 256 / max512 k6 VUs, API max in-flight256, queue timeout5ms, execution deadline1s, Redis admission5000/s.",
			"Client/API/dependencies share WSL hardware; this is the highest verified local tier under SLO, not a production hard maximum. Failed probes and ambiguous persisted responses are retained.",
			"Resources sampled at 1s. CPU 100%=one logical core; host percentage normalized. Metrics observer traffic and preparation excluded from k6 counters."}}}
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
	if err = h.start("mysql-redis", "decision-capacity"); err != nil {
		return
	}
	hashes := h.report["sourceSHA256"].(map[string]string)
	for _, path := range []string{"tests/bench/jmeter-profile/decision_capacity.go", "tests/bench/jmeter-profile/capacity.go", "tests/load/decision-capacity.js", "internal/decision/application/service.go", "internal/decision/application/admission.go", "internal/decision/application/auction.go", "internal/decision/adapter/redis/reservations.go"} {
		b, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		hashes[path] = fmt.Sprintf("%x", sha256.Sum256(b))
	}
	version, e := exec.Command("work/k6-lab/k6-v2.2.0-linux-amd64/k6", "version").CombinedOutput()
	if e != nil {
		return e
	}
	h.report["k6Version"] = strings.TrimSpace(string(version))
	winner, e := h.seedAuction()
	if e != nil {
		return e
	}
	h.report["winningCampaign"] = winner
	lo, hi := 0, 0
	run := func(rate, seconds int, label string) (bool, error) {
		r, e := h.decisionProbe(winner, rate, seconds, label)
		h.results = append(h.results, r)
		h.report["results"] = h.results
		_ = h.save()
		if e != nil {
			return false, e
		}
		return r["passed"].(bool), nil
	}
	for _, rate := range []int{25, 50, 100, 200, 400, 800, 1600, 2000} {
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
		h.report["conclusion"] = "Even the initial decision rate did not pass; inspect errors, no stable tier proven"
		return nil
	}
	for hi > lo+25 {
		mid := ((lo + hi) / 2 / 25) * 25
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
	h.report["shortProbeBracket"] = map[string]int{"passed": lo, "failed": hi}
	for attempt := 0; attempt < 5 && lo > 0; attempt++ {
		all := true
		for repeat := 1; repeat <= 3; repeat++ {
			ok, e := run(lo, confirmSeconds, fmt.Sprintf("confirm-%d", repeat))
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
			h.report["ceilingWithoutFailure"] = hi == 0
			return nil
		}
		hi = lo
		lo -= 25
	}
	h.report["conclusion"] = "No tier achieved three long confirmations within bounded backoff"
	return nil
}
func (h *lab) seedAuction() (string, error) {
	winner := ""
	for _, bid := range []int{2, 3, 5} {
		body, _ := json.Marshal(map[string]any{"name": fmt.Sprintf("Decision load bidder %d", bid), "slotId": "decision-capacity-slot", "startAt": time.Now().UTC().Add(-time.Minute), "endAt": time.Now().UTC().Add(2 * time.Hour)})
		// request helper normally accepts only 200; creation has its own status check.
		raw, e := h.createForDecision("/v1/campaigns", body, 201)
		if e != nil {
			return "", e
		}
		var c struct {
			ID string `json:"id"`
		}
		if e = json.Unmarshal(raw, &c); e != nil {
			return "", e
		}
		body, _ = json.Marshal(map[string]string{"title": "Decision capacity creative", "description": "Isolated test fixture", "imageUrl": "https://example.com/test.png", "landingUrl": "https://example.com"})
		if _, e = h.createForDecision("/v1/campaigns/"+c.ID+"/creatives", body, 201); e != nil {
			return "", e
		}
		body, _ = json.Marshal(map[string]any{"targeting": map[string]any{"all": []any{map[string]string{"tag": "jmeter_test"}}}, "dailyBudgetFen": decisionBudgetFen, "impressionCostFen": bid, "frequencyLimit": 100, "auction": map[string]any{"advertiserId": fmt.Sprintf("decision-bidder-%d", bid), "advertiserName": fmt.Sprintf("Bidder %d", bid), "bidFen": bid}})
		if _, e = h.request("POST", "/v1/campaigns/"+c.ID+"/publish", body); e != nil {
			return "", e
		}
		if bid == 5 {
			winner = c.ID
		}
	}
	return winner, nil
}
func (h *lab) decisionProbe(winner string, rate, seconds int, label string) (r map[string]any, err error) {
	id := fmt.Sprintf("%02d-%s-%d", len(h.results)+1, label, rate)
	r = map[string]any{"id": id, "label": label, "offeredQPS": rate, "seconds": seconds}
	fmt.Printf("START decision %s for %ds\n", id, seconds)
	if err = h.rc.FlushDB(context.Background()).Err(); err != nil {
		return
	}
	for i := 0; i < 1000; i++ {
		if _, err = h.request("GET", fmt.Sprintf("/v1/profiles/profile-%04d", i), nil); err != nil {
			return
		}
	}
	for i := 0; i < 3; i++ {
		body, _ := json.Marshal(map[string]string{"requestId": fmt.Sprintf("dc-%s-warm-%d", id, i), "userId": fmt.Sprintf("profile-%04d", i), "slotId": "decision-capacity-slot"})
		raw, e := h.request("POST", "/v1/decisions", body)
		if e != nil {
			err = e
			return
		}
		var result struct {
			Matched  bool   `json:"matched"`
			Campaign string `json:"campaignId"`
		}
		if e = json.Unmarshal(raw, &result); e != nil {
			err = e
			return
		}
		if !result.Matched || result.Campaign != winner {
			err = fmt.Errorf("warmup failed auction: %s", raw)
			return
		}
	}
	fetchBefore, e := h.tableFetches()
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
	args := []string{"run", "--quiet", "--no-color", "--no-usage-report", "-e", "BASE_URL=" + h.base, "-e", "STAGE=" + id, "-e", "WINNER=" + winner, "-e", fmt.Sprintf("RATE=%d", rate), "-e", fmt.Sprintf("SECONDS=%d", seconds), "-e", "REPORT_PATH=" + rawPath, "tests/load/decision-capacity.js"}
	cmd := exec.Command("work/k6-lab/k6-v2.2.0-linux-amd64/k6", args...)
	cmd.Env = append(os.Environ(), "ADFLOW_CAPACITY_TOKEN="+h.token)
	if err = launch(cmd, filepath.Join(h.dir, id+"-console.log")); err != nil {
		return
	}
	resources, runErr := h.observeDecisionProcess(cmd)
	if runErr != nil {
		r["k6Exit"] = runErr.Error()
	}
	r["resources"] = resources
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
	v := func(name string) map[string]float64 { return summary.Metrics[name].Values }
	requests, matched := v("http_reqs")["count"], v("matched_decisions")["count"]
	p95, errorRate, dropped := v("http_req_duration")["p(95)"], v("decision_errors")["rate"], v("dropped_iterations")["count"]
	r["requests"] = requests
	r["matched"] = matched
	r["noAd"] = v("no_ad")["count"]
	r["unexpectedAuction"] = v("unexpected_auction")["count"]
	r["httpErrors"] = v("decision_http_errors")["count"]
	r["httpMetrics"] = v("http_req_duration")
	r["actualQPS"] = v("http_reqs")["rate"]
	r["errorRate"] = errorRate
	r["dropped"] = dropped
	var stored, storedMatched, wrong, wrongUser int64
	if err = h.db.QueryRow("SELECT COUNT(*),COALESCE(SUM(matched=1),0),COALESCE(SUM(matched=1 AND (COALESCE(campaign_id,'')<>? OR COALESCE(JSON_UNQUOTE(JSON_EXTRACT(pricing,'$.priceFen')),'')<>'5')),0),COALESCE(SUM(user_id<>CONCAT('profile-',LPAD(MOD(CAST(SUBSTRING_INDEX(request_id,'-',-1) AS UNSIGNED),1000),4,'0'))),0) FROM decisions WHERE request_id LIKE ?", winner, "dc-"+id+"-%").Scan(&stored, &storedMatched, &wrong, &wrongUser); err != nil {
		return
	}
	var heldMap map[string]string
	day := time.Now().UTC().Format("2006-01-02")
	root := "adflow:budget:" + day + ":" + winner
	spent, e := h.rc.Get(context.Background(), root+":spent").Int64()
	if e != nil && e != redis.Nil {
		err = e
		return
	}
	held, e := h.rc.Get(context.Background(), root+":reserved").Int64()
	if e != nil {
		err = e
		return
	}
	heldMap, e = h.rc.HGetAll(context.Background(), root+":amounts").Result()
	if e != nil {
		err = e
		return
	}
	var sum int64
	for _, value := range heldMap {
		n, e := strconv.ParseInt(value, 10, 64)
		if e != nil {
			err = e
			return
		}
		sum += n
	}
	maxFrequency := int64(0)
	for i := 0; i < 1000; i++ {
		n, e := h.rc.ZCard(context.Background(), fmt.Sprintf("adflow:freq:%s:%s:profile-%04d", day, winner, i)).Result()
		if e != nil {
			err = e
			return
		}
		if n > maxFrequency {
			maxFrequency = n
		}
	}
	var events int64
	if err = h.db.QueryRow("SELECT COUNT(*) FROM event_receipts").Scan(&events); err != nil {
		return
	}
	proof := storedMatched >= int64(matched)+3 && wrong == 0 && wrongUser == 0 && spent == 0 && held == sum && held <= decisionBudgetFen && maxFrequency <= 100 && events == 0
	r["proof"] = map[string]any{"storedDecisionsIncludingWarmup": stored, "storedMatchedIncludingWarmup": storedMatched, "warmupDecisions": 3, "wrongStoredWinnerOrPrice": wrong, "wrongStoredUser": wrongUser, "spentFen": spent, "reservedFen": held, "reservationAmountSumFen": sum, "reservationEntries": len(heldMap), "maxUserFrequency": maxFrequency, "eventReceipts": events, "consistent": proof}
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
	for k, n := range cacheBefore {
		cacheAfter[k] -= n
	}
	r["tableFetches"] = after - fetchBefore
	r["cacheCounterDeltas"] = cacheAfter
	r["passed"] = runErr == nil && p95 < 300 && errorRate < 0.001 && dropped == 0 && requests >= float64(rate*seconds)*0.99 && matched >= requests*.999 && r["noAd"] == float64(0) && r["unexpectedAuction"] == float64(0) && proof
	fmt.Printf("END decision %s: actual %.1f/s p95 %.2fms matched %.0f/%.0f errors %.4f dropped %.0f pass=%v\n", id, v("http_reqs")["rate"], p95, matched, requests, errorRate, dropped, r["passed"])
	return
}
func (h *lab) observeDecisionProcess(cmd *exec.Cmd) ([]map[string]any, error) {
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	pids := map[string]int{"api": h.api.Process.Pid, "injector": cmd.Process.Pid, "redis": h.rp.Process.Pid}
	if raw, e := exec.Command("pidof", "mysqld").Output(); e == nil {
		fields := strings.Fields(string(raw))
		if len(fields) > 0 {
			n, _ := strconv.Atoi(fields[0])
			pids["mysql"] = n
		}
	}
	clock := float64(100)
	if raw, e := exec.Command("getconf", "CLK_TCK").Output(); e == nil {
		if n, e := strconv.ParseFloat(strings.TrimSpace(string(raw)), 64); e == nil && n > 0 {
			clock = n
		}
	}
	before := map[string]procReading{}
	for name, pid := range pids {
		before[name] = readProc(pid)
	}
	hostTotal, hostIdle := readHost()
	last := time.Now()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var points []map[string]any
	for {
		select {
		case err := <-done:
			return points, err
		case <-ticker.C:
			now := time.Now()
			elapsed := now.Sub(last).Seconds()
			point := map[string]any{"at": now.UTC()}
			for name, pid := range pids {
				v := readProc(pid)
				old := before[name]
				if v.Ticks >= old.Ticks && v.RSS > 0 {
					point[name] = map[string]any{"cpuPercentOneCore": float64(v.Ticks-old.Ticks) / clock / elapsed * 100, "rssBytes": v.RSS, "threads": v.Threads}
				}
				before[name] = v
			}
			total, idle := readHost()
			if total > hostTotal {
				point["hostBusyPercent"] = 100 * (1 - float64(idle-hostIdle)/float64(total-hostTotal))
			}
			hostTotal, hostIdle = total, idle
			if raw, e := h.request("GET", "/metrics", nil); e == nil {
				for _, line := range strings.Split(string(raw), "\n") {
					for _, name := range []string{"go_goroutines", "go_sched_gomaxprocs_threads", "adflow_decision_in_flight"} {
						if strings.HasPrefix(line, name+" ") {
							n, _ := strconv.ParseFloat(strings.TrimPrefix(line, name+" "), 64)
							point[name] = n
						}
					}
				}
			}
			points = append(points, point)
			last = now
		}
	}
}
