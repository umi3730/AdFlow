// Full HTTP -> auction -> Redis settlement -> Outbox -> Kafka -> MySQL proof.
// Run on Linux/WSL as a local test harness, never against an application schema.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
	"github.com/zhanghaiyang/adflow/internal/platform/migrate"
)

type sample struct {
	ID                  string           `json:"requestId"`
	User                string           `json:"userId"`
	Start               time.Time        `json:"startedAt"`
	DecisionMS          float64          `json:"decisionMs"`
	AcceptMS            float64          `json:"allEventsAcceptedMs"`
	PersistMS           float64          `json:"allEventsProcessedMs"`
	ProcessedAt         *time.Time       `json:"lastProcessedAt,omitempty"`
	CommittedObservedMS float64          `json:"committedObservedMs"`
	ObservedAt          *time.Time       `json:"committedObservedAt,omitempty"`
	Error               string           `json:"error,omitempty"`
	Warm                bool             `json:"warmup"`
	Winner              string           `json:"winner"`
	Creative            string           `json:"creativeId"`
	Events              []map[string]any `json:"events"`
	Codes               []int            `json:"httpStatuses"`
}
type groupResult struct {
	Scenario string         `json:"scenario"`
	Mode     string         `json:"indexMode"`
	Round    int            `json:"round"`
	ID       string         `json:"runId"`
	Started  time.Time      `json:"startedAt"`
	Elapsed  float64        `json:"dispatchToDrainSeconds"`
	Samples  []sample       `json:"samples"`
	Summary  map[string]any `json:"summary"`
	Proof    map[string]any `json:"proof"`
	Failures []string       `json:"failures"`
}
type harness struct {
	db                                                                             *sql.DB
	admin                                                                          *sql.DB
	redis                                                                          *redis.Client
	api                                                                            *exec.Cmd
	redisProcess                                                                   *exec.Cmd
	root, dir, schema, topic, dlq, group, redisAddr, base, token, binary, kafkaBin string
	http                                                                           *http.Client
	report                                                                         map[string]any
	results                                                                        []groupResult
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "full-chain failed:", err)
		os.Exit(1)
	}
}
func run() (err error) {
	rows := flag.Int("rows", 1000000, "historical events")
	count := flag.Int("requests", 120, "measured requests per arm")
	rounds := flag.Int("rounds", 3, "paired rounds per scenario")
	rate := flag.Int("rate", 10, "scheduled rounds/second")
	binary := flag.String("api", "work/end-to-end/api-linux", "built production API executable")
	output := flag.String("output", "work/end-to-end/latest", "new output directory")
	flag.Parse()
	if *rows < 300 || *rows > 1000000 || *count < 10 || *count > 1000 || *rounds < 1 || *rate < 1 || *rate > 100 {
		return fmt.Errorf("invalid bounded experiment configuration")
	}
	if runtime.GOOS != "linux" {
		return fmt.Errorf("run harness inside Linux/WSL")
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	dir, err := filepath.Abs(*output)
	if err != nil {
		return err
	}
	if err = os.Mkdir(dir, 0755); err != nil {
		return fmt.Errorf("use a new output directory: %w", err)
	}
	stamp := time.Now().UTC().Format("20060102-150405")
	h := &harness{root: root, dir: dir, schema: "adflow_e2ebench_" + strings.ReplaceAll(stamp, "-", "_"), topic: "adflow-e2ebench-" + stamp, dlq: "adflow-e2ebench-dlq-" + stamp, group: "adflow-e2ebench-group-" + stamp, kafkaBin: "/opt/adflow-runtime/kafka_2.13-4.3.1/bin", http: &http.Client{Timeout: 8 * time.Second, Transport: &http.Transport{MaxIdleConns: 64, MaxIdleConnsPerHost: 32, MaxConnsPerHost: 32}}, report: map[string]any{}}
	h.binary, err = filepath.Abs(*binary)
	if err != nil {
		return err
	}
	h.report = map[string]any{"startedAt": time.Now().UTC(), "requestsPerArm": *count, "pairedRounds": *rounds, "targetRoundsPerSecond": *rate, "historicalRows": *rows, "maxClientInFlight": 20, "results": []groupResult{}, "sourceSHA256": map[string]string{}, "notes": []string{
		"Production API executable; auth enabled, MySQL campaigns/profiles/decisions/audit, Redis cache/admission/reservations and real Kafka. No mock decision/event adapters.",
		"HTTP client, API and MySQL colocated in WSL; background services share host resources. Not production capacity or a maximum QPS test.",
		"Each arm uses fresh campaigns, saved users and request IDs. Three advertisers bid 2/3/5 fen for the same segment. All measured users must match the 5-fen winner; no No-Ad samples dilute workload.",
		"Profiles are prewarmed through HTTP; 3 complete warmup flows excluded from latency summaries but included in correctness/accounting totals.",
		"Paired AB/BA index visibility on same million-row fixture. Earlier measured events remain, all verified published/processed before next arm; new rows are reported, not silently reset.",
		"healthy_history: all synthetic history PUBLISHED. stranded_backlog: ~5% static SETTLING rows without settlement tasks; deliberately stranded stress fixture, not normal healthy production backlog.",
		"All accepted HTTP event calls commit real database transactions. Completion time uses final processed_events timestamp minus client decision start (same host UTC); observing SELECT occurs only after dispatch and is not included in stored processing timestamps.",
		"Both index modes physically maintain the index; only optimizer visibility changes. No application timeout, worker or pool setting changes between arms.",
		"Database processed_at is assigned before commit. committedObservedMs is measured by a concurrent observer when all 3 committed rows are first visible; includes up to a polling interval (100ms) plus query/scheduling overhead. No latency claim treats the stored timestamp as commit time.",
	}}
	defer func() {
		if err != nil {
			h.report["error"] = err.Error()
		}
		cleanup := h.cleanup()
		h.report["cleanup"] = cleanup
		h.report["finishedAt"] = time.Now().UTC()
		_ = h.save()
	}()
	if err = h.setup(*rows); err != nil {
		return err
	}
	for _, scenario := range []string{"healthy_history", "stranded_backlog"} {
		if scenario == "stranded_backlog" {
			fmt.Println("Preparing static 5% stranded SETTLING fixture")
			if _, err = h.db.Exec(`UPDATE event_outbox SET status='SETTLING' WHERE event_id LIKE 'hist-%' AND MOD(CAST(SUBSTRING(aggregate_key,6) AS UNSIGNED),100)>=95`); err != nil {
				return err
			}
		}
		for round := 1; round <= *rounds; round++ {
			modes := []string{"invisible", "visible"}
			if round%2 == 0 {
				modes = []string{"visible", "invisible"}
			}
			for _, mode := range modes {
				fmt.Printf("Running %s round=%d index=%s (%d rounds @ %d/s)\n", scenario, round, mode, *count, *rate)
				if _, err = h.db.Exec("ALTER TABLE event_outbox ALTER INDEX idx_outbox_request_status " + strings.ToUpper(mode)); err != nil {
					return err
				}
				var result groupResult
				result, err = h.measure(scenario, mode, round, *count, *rate)
				h.results = append(h.results, result)
				h.report["results"] = h.results
				_ = h.save()
				if err != nil {
					return err
				}
				fmt.Printf("DONE %s %s round=%d %s\n", scenario, mode, round, mustJSON(result.Summary))
				if len(result.Failures) > 0 {
					fmt.Println("Correctness failures retained:", result.Failures)
				}
			}
		}
	}
	return nil
}
func (h *harness) save() error {
	raw, err := json.MarshalIndent(h.report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(h.dir, "results.json"), raw, 0644)
}
func mustJSON(v any) string { raw, _ := json.Marshal(v); return string(raw) }
func freePort() (string, error) {
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		return "", e
	}
	defer l.Close()
	return l.Addr().String(), nil
}
func (h *harness) setup(n int) error {
	cfg := driver.NewConfig()
	cfg.User = "root"
	cfg.Net = "unix"
	cfg.Addr = "/var/run/mysqld/mysqld.sock"
	cfg.ParseTime = true
	cfg.InterpolateParams = true
	cfg.Loc = time.UTC
	cfg.Params = map[string]string{"time_zone": "'+00:00'"}
	var err error
	h.admin, err = sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return err
	}
	if _, err = h.admin.Exec("CREATE DATABASE `" + h.schema + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		return err
	}
	cfg.DBName = h.schema
	h.db, err = sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return err
	}
	h.db.SetMaxOpenConns(4)
	runner, err := migrate.NewRunner(h.db, "migrations")
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	versions, err := runner.Up(ctx)
	if err != nil {
		return err
	}
	h.report["migrations"] = versions
	hashes := h.report["sourceSHA256"].(map[string]string)
	for _, path := range []string{"tests/bench/full-chain/main.go", "cmd/api/main.go", "internal/event/application/settlement_worker.go", "internal/event/adapter/mysql/settlement.go", "internal/event/adapter/mysql/outbox.go", "internal/event/adapter/mysql/store.go", "internal/decision/application/service.go", "internal/decision/adapter/redis/settlement.go", "migrations/000011_outbox_request_index.up.sql"} {
		raw, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		hashes[path] = fmt.Sprintf("%x", sha256.Sum256(raw))
	}
	raw, err := os.ReadFile(h.binary)
	if err != nil {
		return err
	}
	h.report["apiBinarySHA256"] = fmt.Sprintf("%x", sha256.Sum256(raw))
	var version string
	var pool int64
	if err = h.db.QueryRow("SELECT VERSION(),@@innodb_buffer_pool_size").Scan(&version, &pool); err != nil {
		return err
	}
	h.report["environment"] = map[string]any{"mysql": version, "innodbBufferPoolBytes": pool, "go": runtime.Version(), "platform": runtime.GOOS + "/" + runtime.GOARCH, "transport": "HTTP loopback; MySQL Unix socket; Redis/Kafka loopback", "apiDecisionTimeout": "1s", "apiProfileCacheTimeout": "50ms", "auth": true, "audit": "mysql", "redisAdmissionLimitPerSecond": 5000, "apiMaxInFlight": 256, "sqlPoolMaxOpen": 30, "kafkaPartitions": 3, "schema": h.schema}
	fmt.Printf("Seeding %d historical published events in %s\n", n, h.schema)
	if err = h.seed(n); err != nil {
		return err
	}
	if h.redisAddr, err = freePort(); err != nil {
		return err
	}
	_, port, _ := net.SplitHostPort(h.redisAddr)
	h.redisProcess = exec.Command("redis-server", "--bind", "127.0.0.1", "--port", port, "--save", "", "--appendonly", "no")
	if err = h.launch(h.redisProcess, "redis.log"); err != nil {
		return err
	}
	h.redis = redis.NewClient(&redis.Options{Addr: h.redisAddr, Protocol: 2, DisableIdentity: true})
	for i := 0; i < 50; i++ {
		if h.redis.Ping(context.Background()).Err() == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err = h.redis.Ping(context.Background()).Err(); err != nil {
		return err
	}
	for _, topic := range []string{h.topic, h.dlq} {
		if _, err = h.kafka("kafka-topics.sh", "--bootstrap-server", "127.0.0.1:9092", "--create", "--topic", topic, "--partitions", "3", "--replication-factor", "1"); err != nil {
			return err
		}
	}
	for _, result := range h.results {
		if len(result.Failures) > 0 {
			return fmt.Errorf("one or more arms failed correctness; see retained report")
		}
	}
	return nil
}
func (h *harness) seed(n int) error {
	for _, s := range []string{"CREATE TABLE seed_digits(n INT PRIMARY KEY)", "INSERT INTO seed_digits VALUES(0),(1),(2),(3),(4),(5),(6),(7),(8),(9)"} {
		if _, err := h.db.Exec(s); err != nil {
			return err
		}
	}
	_, err := h.db.Exec(`INSERT INTO event_receipts(event_id,request_id,campaign_id,creative_id,event_type,occurred_at)
 SELECT CONCAT('hist-',LPAD(n,12,'0')),CONCAT('hist-',LPAD(FLOOR(n/3),12,'0')),REPEAT('0',32),REPEAT('1',32),
 ELT(MOD(n,3)+1,'impression','click','conversion'),'2026-09-01 00:00:00'
 FROM(SELECT a.n+10*b.n+100*c.n+1000*d.n+10000*e.n+100000*f.n n FROM seed_digits a CROSS JOIN seed_digits b CROSS JOIN seed_digits c CROSS JOIN seed_digits d CROSS JOIN seed_digits e CROSS JOIN seed_digits f)nums WHERE n<? ORDER BY n`, n)
	if err != nil {
		return err
	}
	_, err = h.db.Exec(`INSERT INTO event_outbox(event_id,aggregate_key,payload,status,published_at)
 SELECT event_id,request_id,JSON_OBJECT('eventId',event_id,'requestId',request_id,'campaignId',campaign_id,'creativeId',creative_id,'type',event_type,'occurredAt','2026-09-01T00:00:00Z'),'PUBLISHED','2026-09-01 00:00:00' FROM event_receipts ORDER BY event_id`)
	if err != nil {
		return err
	}
	_, err = h.db.Exec("ANALYZE TABLE event_outbox")
	return err
}
func (h *harness) launch(cmd *exec.Cmd, name string) error {
	f, e := os.Create(filepath.Join(h.dir, name))
	if e != nil {
		return e
	}
	defer f.Close()
	cmd.Stdout = f
	cmd.Stderr = f
	return cmd.Start()
}
func stop(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case e := <-done:
		return e
	case <-time.After(15 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		return fmt.Errorf("forced stop")
	}
}
func (h *harness) kafka(script string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(h.kafkaBin, script), args...)
	cmd.Env = append(os.Environ(), "KAFKA_HEAP_OPTS=-Xmx128m -Xms64m")
	out, e := cmd.CombinedOutput()
	return string(out), e
}
func (h *harness) startAPI(id string) error {
	addr, err := freePort()
	if err != nil {
		return err
	}
	h.base = "http://" + addr
	h.token = ""
	cmd := exec.Command(h.binary)
	cmd.Dir = h.root
	// Explicit config, without inheriting application credentials/provider keys.
	env := []string{}
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "ADFLOW_") {
			env = append(env, v)
		}
	}
	cfg := driver.NewConfig()
	cfg.User = "root"
	cfg.Net = "unix"
	cfg.Addr = "/var/run/mysqld/mysqld.sock"
	cfg.DBName = h.schema
	cfg.ParseTime = true
	settings := map[string]string{"ENV": "test", "HTTP_ADDR": addr, "MYSQL_DSN": cfg.FormatDSN(), "REDIS_ADDR": h.redisAddr, "CAMPAIGN_REPOSITORY": "mysql", "PROFILE_STORE": "mysql-redis", "DECISION_STORE": "mysql", "RESERVATION_ADAPTER": "redis", "EVENT_TRANSPORT": "kafka", "KAFKA_BROKERS": "127.0.0.1:9092", "KAFKA_TOPIC": h.topic, "KAFKA_DEAD_LETTER_TOPIC": h.dlq, "KAFKA_CONSUMER_GROUP": h.group, "AUTH_ENABLED": "true", "AUDIT_STORE": "mysql", "DEMO_DATA": "false", "AGENT_PROVIDER": "mock", "DECISION_RATE_LIMITER": "redis", "DECISION_TIMEOUT": "1s", "PROFILE_CACHE_TIMEOUT": "50ms"}
	for k, v := range settings {
		env = append(env, "ADFLOW_"+k+"="+v)
	}
	cmd.Env = env
	h.api = cmd
	if err = h.launch(cmd, id+"-api.log"); err != nil {
		return err
	}
	for i := 0; i < 80; i++ {
		var body map[string]any
		code, _, e := h.call("GET", "/readyz", nil, &body)
		if e == nil && code == 200 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	var login struct {
		AccessToken string `json:"accessToken"`
	}
	code, _, err := h.call("POST", "/v1/auth/login", map[string]string{"username": "admin", "password": "adflow-admin"}, &login)
	if err != nil || code != 200 || login.AccessToken == "" {
		return fmt.Errorf("isolated API login status %d: %v", code, err)
	}
	h.token = login.AccessToken
	return nil
}
func (h *harness) call(method, path string, body, out any) (int, float64, error) {
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return 0, 0, err
		}
	}
	req, err := http.NewRequest(method, h.base+path, bytes.NewReader(data))
	if err != nil {
		return 0, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if h.token != "" {
		req.Header.Set("Authorization", "Bearer "+h.token)
	}
	start := time.Now()
	res, err := h.http.Do(req)
	if err != nil {
		return 0, float64(time.Since(start)) / float64(time.Millisecond), err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	ms := float64(time.Since(start)) / float64(time.Millisecond)
	if err != nil {
		return res.StatusCode, ms, err
	}
	if res.StatusCode >= 400 {
		return res.StatusCode, ms, fmt.Errorf("%s %s: HTTP %d %s", method, path, res.StatusCode, string(raw))
	}
	if out != nil && len(raw) > 0 {
		err = json.Unmarshal(raw, out)
	}
	return res.StatusCode, ms, err
}
func (h *harness) measure(scenario, mode string, round, count, rate int) (g groupResult, err error) {
	id := fmt.Sprintf("%s-%s-%d", scenario, mode, round)
	g = groupResult{Scenario: scenario, Mode: mode, Round: round, ID: id, Proof: map[string]any{}}
	if err = h.startAPI(id); err != nil {
		return
	}
	defer func() {
		e := stop(h.api)
		h.api = nil
		if err == nil {
			err = e
		}
	}()
	var winner string
	var campaignIDs []string
	slot := "e2e-" + id
	for _, bid := range []int{2, 3, 5} {
		var campaign struct {
			ID string `json:"id"`
		}
		_, _, err = h.call("POST", "/v1/campaigns", map[string]any{"name": fmt.Sprintf("E2E %s bid%d", id, bid), "slotId": slot, "startAt": time.Now().UTC().Add(-time.Minute), "endAt": time.Now().UTC().Add(time.Hour)}, &campaign)
		if err != nil {
			return
		}
		campaignIDs = append(campaignIDs, campaign.ID)
		_, _, err = h.call("POST", "/v1/campaigns/"+campaign.ID+"/creatives", map[string]any{"title": "E2E creative", "description": "Synthetic fixture", "imageUrl": "https://example.com/test.png", "landingUrl": "https://example.com"}, nil)
		if err != nil {
			return
		}
		_, _, err = h.call("POST", "/v1/campaigns/"+campaign.ID+"/publish", map[string]any{"targeting": map[string]any{"all": []any{map[string]string{"tag": id}}}, "dailyBudgetFen": 10000000, "impressionCostFen": bid, "frequencyLimit": 100, "auction": map[string]any{"advertiserId": fmt.Sprintf("e2e-%d", bid), "advertiserName": fmt.Sprintf("Advertiser %d", bid), "bidFen": bid}}, nil)
		if err != nil {
			return
		}
		if bid == 5 {
			winner = campaign.ID
		}
	}
	users := make([]string, count+3)
	for i := range users {
		users[i] = fmt.Sprintf("e2e-%s-%04d", id, i)
		_, _, err = h.call("PUT", "/v1/profiles/"+users[i], map[string]any{"tags": []string{id}, "fields": map[string]string{"device": "android", "age": "25", "score": "88"}}, nil)
		if err != nil {
			return
		}
		_, _, err = h.call("GET", "/v1/profiles/"+users[i], nil, nil)
		if err != nil {
			return
		}
	}
	for i := 0; i < 3; i++ {
		g.Samples = append(g.Samples, h.flow(users[i], slot, id, i, winner, true))
	}
	if err = h.drain(g.Samples, 30*time.Second); err != nil {
		return
	}
	measured := make([]sample, count)
	var wg sync.WaitGroup
	slots := make(chan struct{}, 20)
	g.Started = time.Now().UTC()
	start := time.Now()
	observed := make(chan observation, 1)
	go func() { observed <- h.observe("e2e-"+id+"-%", count+3, time.Duration(count/rate+90)*time.Second) }()
	for i := 0; i < count; i++ {
		due := start.Add(time.Duration(i) * time.Second / time.Duration(rate))
		if delay := time.Until(due); delay > 0 {
			time.Sleep(delay)
		}
		slots <- struct{}{}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			defer func() { <-slots }()
			measured[i] = h.flow(users[i+3], slot, id, i+3, winner, false)
		}(i)
	}
	wg.Wait()
	g.Samples = append(g.Samples, measured...)
	finished := <-observed
	if finished.Error != "" {
		g.Failures = append(g.Failures, finished.Error)
	}
	for i := range g.Samples {
		if at, ok := finished.Rows[g.Samples[i].ID]; ok {
			g.Samples[i].ProcessedAt = &at.Recorded
			g.Samples[i].ObservedAt = &at.Observed
			g.Samples[i].PersistMS = float64(at.Recorded.Sub(g.Samples[i].Start)) / float64(time.Millisecond)
			g.Samples[i].CommittedObservedMS = float64(at.Observed.Sub(g.Samples[i].Start)) / float64(time.Millisecond)
		}
	}
	g.Elapsed = time.Since(start).Seconds()
	g.Summary = summarize(g.Samples, g.Elapsed)
	// Replay identical request/events after processing; it must not add money/events.
	first := g.Samples[3]
	var duplicate map[string]any
	code, _, e := h.call("POST", "/v1/decisions", map[string]string{"requestId": first.ID, "userId": first.User, "slotId": slot}, &duplicate)
	duplicateOK := e == nil && code == 200 && duplicate["campaignId"] == winner
	for _, event := range first.Events {
		var response struct {
			Recorded bool `json:"recorded"`
		}
		code, _, e = h.call("POST", "/v1/events", event, &response)
		duplicateOK = duplicateOK && e == nil && code == 200 && !response.Recorded
	}
	g.Proof["duplicateRequestAndEventsIdempotent"] = duplicateOK
	if !duplicateOK {
		g.Failures = append(g.Failures, "duplicate replay changed response/failed")
	}
	expected := int64(count + 3)
	var impressions, clicks, conversions, value int64
	err = h.db.QueryRow("SELECT impressions,clicks,conversions,value_fen FROM campaign_metrics WHERE campaign_id=?", winner).Scan(&impressions, &clicks, &conversions, &value)
	if err != nil {
		return
	}
	g.Proof["metrics"] = map[string]any{"impressions": impressions, "clicks": clicks, "conversions": conversions, "valueFen": value, "expectedEach": expected, "expectedValueFen": expected * 500}
	if impressions != expected || clicks != expected || conversions != expected || value != expected*500 {
		g.Failures = append(g.Failures, "final metrics mismatch")
	}
	prefix := "e2e-" + id + "-%"
	var settled, published, processed, receipts int64
	for query, target := range map[string]*int64{
		"SELECT COUNT(*) FROM event_settlements WHERE request_id LIKE ? AND status='SETTLED'": &settled,
		"SELECT COUNT(*) FROM event_outbox WHERE aggregate_key LIKE ? AND status='PUBLISHED'": &published,
		"SELECT COUNT(*) FROM processed_events WHERE request_id LIKE ?":                       &processed,
		"SELECT COUNT(*) FROM event_receipts WHERE request_id LIKE ?":                         &receipts,
	} {
		if err = h.db.QueryRow(query, prefix).Scan(target); err != nil {
			return
		}
	}
	g.Proof["durableCounts"] = map[string]any{"settled": settled, "published": published, "processed": processed, "receipts": receipts, "expectedSettlements": expected, "expectedEvents": expected * 3}
	if settled != expected || published != expected*3 || processed != expected*3 || receipts != expected*3 {
		g.Failures = append(g.Failures, "durable counts mismatch")
	}
	day := g.Started.UTC().Format("2006-01-02")
	root := "adflow:budget:" + day + ":" + winner
	spent, _ := h.redis.Get(context.Background(), root+":spent").Int64()
	reserved, _ := h.redis.Get(context.Background(), root+":reserved").Int64()
	frequencyOK := true
	for _, user := range users {
		n, e := h.redis.ZCard(context.Background(), "adflow:freq:"+day+":"+winner+":"+user).Result()
		if e != nil || n != 1 {
			frequencyOK = false
		}
	}
	g.Proof["redis"] = map[string]any{"spentFen": spent, "expectedSpentFen": expected * 5, "reservedFen": reserved, "allUsersFrequencyOne": frequencyOK}
	if spent != expected*5 || reserved != 0 || !frequencyOK {
		g.Failures = append(g.Failures, "budget/frequency mismatch")
	}
	for _, campaign := range campaignIDs {
		if campaign == winner {
			continue
		}
		var total int64
		if err = h.db.QueryRow("SELECT COUNT(*) FROM campaign_metrics WHERE campaign_id=? AND (impressions>0 OR clicks>0 OR conversions>0)", campaign).Scan(&total); err != nil {
			return
		}
		if total != 0 {
			g.Failures = append(g.Failures, "losing advertiser received metrics")
		}
	}
	var trace map[string]any
	_, _, err = h.call("GET", "/v1/operations/request-trace?requestId="+first.ID, nil, &trace)
	if err != nil {
		return
	}
	g.Proof["exampleTrace"] = trace
	lagOK := false
	var lag map[string]any
	for i := 0; i < 30; i++ {
		_, _, e = h.call("GET", "/v1/operations/kafka-lag", nil, &lag)
		if e == nil {
			items, _ := lag["items"].([]any)
			lagOK = len(items) == 3
			for _, item := range items {
				entry := item.(map[string]any)
				if entry["lag"] != float64(0) {
					lagOK = false
				}
			}
		}
		if lagOK {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	g.Proof["kafkaLag"] = lag
	g.Proof["kafkaThreePartitionsZeroLag"] = lagOK
	if !lagOK {
		g.Failures = append(g.Failures, "Kafka lag not zero/unknown")
	}
	var statuses []map[string]any
	rs, e := h.db.Query("SELECT status,COUNT(*) FROM event_outbox WHERE aggregate_key LIKE ? GROUP BY status", prefix)
	if e != nil {
		err = e
		return
	}
	for rs.Next() {
		var s string
		var n int64
		if err = rs.Scan(&s, &n); err != nil {
			rs.Close()
			return
		}
		statuses = append(statuses, map[string]any{"status": s, "count": n})
	}
	rs.Close()
	g.Proof["runOutboxStatuses"] = statuses
	return
}
func (h *harness) flow(user, slot, run string, i int, winner string, warm bool) (s sample) {
	s = sample{ID: fmt.Sprintf("e2e-%s-%04d", run, i), User: user, Start: time.Now().UTC(), Warm: warm}
	var d struct {
		Matched    bool   `json:"matched"`
		CampaignID string `json:"campaignId"`
		CreativeID string `json:"creativeId"`
		Pricing    struct {
			Mode        string `json:"mode"`
			PriceFen    int    `json:"priceFen"`
			Advertisers int    `json:"advertisers"`
		} `json:"pricing"`
	}
	code, ms, err := h.call("POST", "/v1/decisions", map[string]string{"requestId": s.ID, "userId": user, "slotId": slot}, &d)
	s.DecisionMS = ms
	s.Codes = append(s.Codes, code)
	if err != nil {
		s.Error = err.Error()
		return
	}
	if !d.Matched || d.CampaignID != winner || d.Pricing.Mode != "first_price" || d.Pricing.PriceFen != 5 {
		s.Error = "unexpected auction result: " + mustJSON(d)
		return
	}
	s.Winner = d.CampaignID
	s.Creative = d.CreativeID
	for _, kind := range []string{"impression", "click", "conversion"} {
		body := map[string]any{"eventId": kind + "-" + s.ID, "requestId": s.ID, "campaignId": s.Winner, "creativeId": s.Creative, "type": kind}
		if kind == "conversion" {
			body["valueFen"] = 500
		}
		s.Events = append(s.Events, body)
		code, _, err = h.call("POST", "/v1/events", body, nil)
		s.Codes = append(s.Codes, code)
		if err != nil {
			s.Error = err.Error()
			return
		}
		if code != 202 {
			s.Error = fmt.Sprintf("new event expected 202 got %d", code)
			return
		}
	}
	s.AcceptMS = float64(time.Since(s.Start)) / float64(time.Millisecond)
	return
}
func (h *harness) drain(samples []sample, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		args := []any{}
		positions := map[string]int{}
		for i, s := range samples {
			if s.ProcessedAt == nil && s.Error == "" {
				positions[s.ID] = i
				args = append(args, s.ID)
			}
		}
		if len(args) == 0 {
			return nil
		}
		rows, err := h.db.Query("SELECT request_id,COUNT(*),MAX(processed_at) FROM processed_events WHERE request_id IN ("+strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")+") GROUP BY request_id", args...)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id string
			var count int
			var at time.Time
			if err = rows.Scan(&id, &count, &at); err != nil {
				rows.Close()
				return err
			}
			if count == 3 {
				i := positions[id]
				samples[i].ProcessedAt = &at
				samples[i].PersistMS = float64(at.Sub(samples[i].Start)) / float64(time.Millisecond)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("async drain timed out with %d pending requests", len(args))
		}
		time.Sleep(100 * time.Millisecond)
	}
}

type completion struct{ Recorded, Observed time.Time }
type observation struct {
	Rows  map[string]completion
	Error string
}

func (h *harness) observe(prefix string, expected int, timeout time.Duration) observation {
	result := observation{Rows: map[string]completion{}}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		rows, err := h.db.Query("SELECT request_id,COUNT(*),MAX(processed_at) FROM processed_events WHERE request_id LIKE ? GROUP BY request_id", prefix)
		if err != nil {
			result.Error = err.Error()
			return result
		}
		for rows.Next() {
			var id string
			var n int
			var recorded time.Time
			if err = rows.Scan(&id, &n, &recorded); err != nil {
				rows.Close()
				result.Error = err.Error()
				return result
			}
			if n == 3 {
				if _, exists := result.Rows[id]; !exists {
					result.Rows[id] = completion{Recorded: recorded, Observed: time.Now().UTC()}
				}
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			result.Error = err.Error()
			return result
		}
		if len(result.Rows) == expected {
			return result
		}
		time.Sleep(100 * time.Millisecond)
	}
	result.Error = fmt.Sprintf("commit observer timed out: %d/%d requests", len(result.Rows), expected)
	return result
}
func summarize(samples []sample, elapsed float64) map[string]any {
	d, a, p := []float64{}, []float64{}, []float64{}
	committed := []float64{}
	errors := 0
	for _, s := range samples {
		if s.Warm {
			continue
		}
		d = append(d, s.DecisionMS)
		if s.Error != "" {
			errors++
			continue
		}
		a = append(a, s.AcceptMS)
		if s.ProcessedAt != nil {
			p = append(p, s.PersistMS)
		}
		if s.ObservedAt != nil {
			committed = append(committed, s.CommittedObservedMS)
		}
	}
	return map[string]any{"requests": len(d), "httpFailedRounds": errors, "processedRounds": len(p), "decisionMs": dist(d), "allEventsAcceptedMs": dist(a), "allEventsProcessedMs": dist(p), "committedObservedMs": dist(committed), "completedRoundsPerSecondIncludingDrain": float64(len(p)) / elapsed}
}
func dist(v []float64) map[string]any {
	if len(v) == 0 {
		return map[string]any{"n": 0}
	}
	sort.Float64s(v)
	n := len(v)
	med := v[n/2]
	if n%2 == 0 {
		med = (med + v[n/2-1]) / 2
	}
	return map[string]any{"n": n, "median": med, "p95": v[(95*n+99)/100-1], "p99": v[(99*n+99)/100-1], "max": v[n-1]}
}
func (h *harness) cleanup() map[string]any {
	out := map[string]any{}
	if e := stop(h.api); e != nil {
		out["apiStopError"] = e.Error()
	} else {
		out["apiStopped"] = true
	}
	h.api = nil
	if h.redis != nil {
		_ = h.redis.Close()
	}
	if e := stop(h.redisProcess); e != nil {
		out["redisStopError"] = e.Error()
	} else {
		out["dedicatedRedisStopped"] = true
	}
	if h.db != nil {
		_ = h.db.Close()
	}
	if h.admin != nil {
		if strings.HasPrefix(h.schema, "adflow_e2ebench_") {
			_, e := h.admin.Exec("DROP DATABASE IF EXISTS `" + h.schema + "`")
			out["schemaDropped"] = e == nil
		}
		_ = h.admin.Close()
	}
	for _, topic := range []string{h.topic, h.dlq} {
		if strings.HasPrefix(topic, "adflow-e2ebench-") {
			_, e := h.kafka("kafka-topics.sh", "--bootstrap-server", "127.0.0.1:9092", "--delete", "--if-exists", "--topic", topic)
			out[topic+"Deleted"] = e == nil
		}
	}
	if strings.HasPrefix(h.group, "adflow-e2ebench-group-") {
		_, e := h.kafka("kafka-consumer-groups.sh", "--bootstrap-server", "127.0.0.1:9092", "--delete", "--group", h.group)
		out["consumerGroupDeleted"] = e == nil
	}
	return out
}
