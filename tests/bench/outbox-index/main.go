// outbox-index measures only the optimizer benefit of migration 000011.
// It creates a new isolated schema; it never accepts an application DB name.
package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	driver "github.com/go-sql-driver/mysql"
)

const selectSQL = `SELECT event_id, status, attempts, next_attempt_at, last_error FROM event_outbox WHERE aggregate_key = ? AND status = 'SETTLING'`

// Kept byte-for-byte aligned with CompleteSettlement's Outbox statement.
const updateSQL = `UPDATE event_outbox SET status = 'PENDING', attempts = 0,
  next_attempt_at = UTC_TIMESTAMP(3), last_error = NULL
  WHERE aggregate_key = ? AND status = 'SETTLING'`

type measurement struct {
	Round        int     `json:"round"`
	Mode         string  `json:"indexMode"`
	Operation    string  `json:"operation"`
	Request      string  `json:"request"`
	ClientMS     float64 `json:"clientStatementMs"`
	ServerMS     float64 `json:"serverStatementMs"`
	LockMS       float64 `json:"statementLockMs"`
	RowsExamined int64   `json:"rowsExamined"`
	RowsAffected int64   `json:"rowsAffected"`
	RowsSent     int64   `json:"rowsSent"`
	Count        int     `json:"verifiedRows"`
}
type distribution struct {
	Median float64 `json:"median"`
	P95    float64 `json:"p95"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
}
type summary struct {
	Mode      string       `json:"indexMode"`
	Operation string       `json:"operation"`
	Samples   int          `json:"samples"`
	ClientMS  distribution `json:"clientStatementMs"`
	ServerMS  distribution `json:"serverStatementMs"`
	Examined  distribution `json:"rowsExamined"`
}
type dataset struct {
	Schema            string                    `json:"schema"`
	Rows              int                       `json:"rows"`
	Counts            map[string]int            `json:"statusCounts"`
	SeedSeconds       float64                   `json:"seedSeconds"`
	IndexBuildSeconds float64                   `json:"indexBuildSeconds"`
	IndexBytes        int64                     `json:"indexBytes"`
	Plans             map[string]map[string]any `json:"plans"`
	Measurements      []measurement             `json:"measurements"`
	Summary           []summary                 `json:"summary"`
	Cleanup           bool                      `json:"schemaDropped"`
}
type report struct {
	Started     string            `json:"startedAt"`
	Finished    string            `json:"finishedAt"`
	Environment map[string]string `json:"environment"`
	Sources     map[string]string `json:"sourceSHA256"`
	Rounds      int               `json:"rounds"`
	Repetitions int               `json:"samplesPerOperationPerRound"`
	Warmups     int               `json:"warmupsPerBlock"`
	SQL         map[string]string `json:"sql"`
	Notes       []string          `json:"notes"`
	Datasets    []dataset         `json:"datasets"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "benchmark failed:", err)
		os.Exit(1)
	}
}
func run() error {
	sizesFlag := flag.String("rows", "100000,1000000", "comma-separated row counts (300..1000000)")
	rounds := flag.Int("rounds", 6, "paired rounds, alternating AB/BA")
	repetitions := flag.Int("samples", 10, "samples per statement per mode in each round")
	warmups := flag.Int("warmups", 3, "unmeasured SELECT+UPDATE pairs per mode/block")
	output := flag.String("output", "work/sql-index/results.json", "raw result path")
	flag.Parse()
	if *rounds < 2 || *repetitions < 2 || *warmups < 1 {
		return fmt.Errorf("use rounds>=2, samples>=2, warmups>=1")
	}
	var sizes []int
	for _, part := range strings.Split(*sizesFlag, ",") {
		n, err := strconv.Atoi(part)
		if err != nil || n < 300 || n > 1000000 {
			return fmt.Errorf("invalid dataset size")
		}
		sizes = append(sizes, n)
	}
	dsn := os.Getenv("ADFLOW_INDEX_BENCH_DSN")
	if dsn == "" {
		dsn = "root@unix(/var/run/mysqld/mysqld.sock)/"
	}
	cfg, err := driver.ParseDSN(dsn)
	if err != nil {
		return fmt.Errorf("invalid benchmark DSN")
	}
	if cfg.DBName != "" {
		return fmt.Errorf("DSN must have no database name; runner creates an isolated schema")
	}
	cfg.InterpolateParams = true
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	if cfg.Params == nil {
		cfg.Params = map[string]string{}
	}
	cfg.Params["time_zone"] = "'+00:00'"
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	var version, isolation, buffer, flush, syncbin, ps string
	err = conn.QueryRowContext(ctx, `SELECT VERSION(), @@transaction_isolation, @@innodb_buffer_pool_size, @@innodb_flush_log_at_trx_commit, @@sync_binlog, @@performance_schema`).Scan(&version, &isolation, &buffer, &flush, &syncbin, &ps)
	if err != nil {
		return err
	}
	if ps != "1" {
		return fmt.Errorf("performance_schema required for actual statement counters")
	}
	var history string
	if err = conn.QueryRowContext(ctx, `SELECT ENABLED FROM performance_schema.setup_consumers WHERE NAME='events_statements_history'`).Scan(&history); err != nil {
		return err
	}
	if history != "YES" {
		return fmt.Errorf("events_statements_history must already be enabled; runner does not change global settings")
	}
	if _, err = conn.ExecContext(ctx, "SET SESSION optimizer_switch='use_invisible_indexes=off'"); err != nil {
		return err
	}
	if _, err = conn.ExecContext(ctx, "SET SESSION transaction_isolation='REPEATABLE-READ'"); err != nil {
		return err
	}
	var threadID int64
	if err = conn.QueryRowContext(ctx, `SELECT THREAD_ID FROM performance_schema.threads WHERE PROCESSLIST_ID=CONNECTION_ID()`).Scan(&threadID); err != nil {
		return err
	}
	r := report{Started: time.Now().UTC().Format(time.RFC3339), Rounds: *rounds, Repetitions: *repetitions, Warmups: *warmups,
		Environment: map[string]string{"mysql": version, "transactionIsolation": "REPEATABLE-READ", "initialIsolation": isolation, "innodbBufferPoolBytes": buffer, "innodbFlushLogAtTrxCommit": flush, "syncBinlog": syncbin, "go": runtime.Version(), "platform": runtime.GOOS + "/" + runtime.GOARCH, "transport": cfg.Net, "driver": "go-sql-driver/mysql v1.9.3; interpolateParams=true", "sqlConcurrency": "1", "optimizerInvisibleIndexes": "off"},
		SQL:         map[string]string{"select": selectSQL, "update": updateSQL}, Sources: map[string]string{},
		Notes: []string{"Synthetic data, three events per request; 90% PUBLISHED, 5% SETTLING, 3% PENDING, 1% PROCESSING, 1% RECONCILE (rounding at dataset tail).",
			"Same physical table/index in both modes; toggle index visibility only. Invisible indexes still incur write maintenance, so this does not measure insert overhead or physical index absence.",
			"Alternating AB/BA rounds, same paired request IDs. Each block warms both statements. This is warmed access-path timing, not a guarantee all rows fit in buffer pool.",
			"UPDATE executes production Outbox statement inside an explicit transaction then ROLLBACK; exactly three rows must change. Timing excludes BEGIN, telemetry queries, ROLLBACK and commit/fsync cost.",
			"SELECT is a diagnostic read of the same predicate/updated columns, not an extra production endpoint. EXPLAIN ANALYZE used only for SELECT; UPDATE uses nonexecuting EXPLAIN FORMAT=JSON.",
			"Actual rows examined and server time come from performance_schema.events_statements_history, not optimizer estimates. Single client; no concurrency/lock-contention or whole-service QPS claim.",
			"Shared local MySQL instance; application schemas are never touched. Dataset schema is dropped after successful measurement; failure retains it for inspection."}}
	for _, path := range []string{"tests/bench/outbox-index/main.go", "migrations/000002_events.up.sql", "migrations/000011_outbox_request_index.up.sql", "internal/event/adapter/mysql/settlement.go"} {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		r.Sources[path] = fmt.Sprintf("%x", sha256.Sum256(data))
		if strings.HasSuffix(path, "settlement.go") && !strings.Contains(string(data), updateSQL) {
			return fmt.Errorf("production SQL changed; update benchmark statement before running")
		}
	}
	for _, n := range sizes {
		d, err := measureDataset(ctx, conn, threadID, n, *rounds, *repetitions, *warmups)
		r.Datasets = append(r.Datasets, d)
		r.Finished = time.Now().UTC().Format(time.RFC3339)
		if saveErr := saveReport(*output, r); saveErr != nil {
			return saveErr
		}
		if err != nil {
			return err
		}
	}
	fmt.Println("Raw report:", *output)
	return nil
}
func saveReport(path string, r report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0644)
}
func measureDataset(ctx context.Context, c *sql.Conn, thread int64, n, rounds, reps, warmups int) (d dataset, err error) {
	d = dataset{Schema: fmt.Sprintf("adflow_idxbench_%s_%d", time.Now().UTC().Format("20060102_150405"), n), Rows: n, Counts: map[string]int{}, Plans: map[string]map[string]any{}}
	// Generated identifier contains only this fixed prefix, UTC digits and size.
	if _, err = c.ExecContext(ctx, "CREATE DATABASE `"+d.Schema+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		return
	}
	fmt.Printf("Seeding %s (%d events)\n", d.Schema, n)
	if _, err = c.ExecContext(ctx, "USE `"+d.Schema+"`"); err != nil {
		return
	}
	raw, readErr := os.ReadFile("migrations/000002_events.up.sql")
	if readErr != nil {
		err = readErr
		return
	}
	for _, ddl := range strings.Split(string(raw), ";") {
		if strings.TrimSpace(ddl) != "" {
			if _, err = c.ExecContext(ctx, ddl); err != nil {
				return
			}
		}
	}
	started := time.Now()
	if _, err = c.ExecContext(ctx, "CREATE TABLE seed_digits (n INT PRIMARY KEY)"); err != nil {
		return
	}
	if _, err = c.ExecContext(ctx, "INSERT INTO seed_digits VALUES (0),(1),(2),(3),(4),(5),(6),(7),(8),(9)"); err != nil {
		return
	}
	// Deterministic IDs/statuses; no sampling or copying of application data.
	seedSQL := `INSERT INTO event_receipts (event_id,request_id,campaign_id,creative_id,event_type,value_fen,occurred_at,status,created_at)
 SELECT CONCAT('evt-',LPAD(n,12,'0')),CONCAT('req-',LPAD(FLOOR(n/3),12,'0')),
 LPAD(MOD(FLOOR(n/3),1000),32,'0'),LPAD(MOD(FLOOR(n/3),1000),32,'0'),
 ELT(MOD(n,3)+1,'impression','click','conversion'),IF(MOD(n,3)=2,500,0),
 TIMESTAMPADD(SECOND,MOD(n,86400),'2026-09-01 00:00:00'),'ACCEPTED','2026-09-01 00:00:00'
 FROM (SELECT a.n+10*b.n+100*c.n+1000*d.n+10000*e.n+100000*f.n AS n
 FROM seed_digits a CROSS JOIN seed_digits b CROSS JOIN seed_digits c CROSS JOIN seed_digits d CROSS JOIN seed_digits e CROSS JOIN seed_digits f) numbers
 WHERE n < ? ORDER BY n`
	if _, err = c.ExecContext(ctx, seedSQL, n); err != nil {
		return
	}
	if _, err = c.ExecContext(ctx, `INSERT INTO event_outbox (event_id,aggregate_key,payload,status,attempts,next_attempt_at,created_at)
 SELECT event_id,request_id,JSON_OBJECT('eventId',event_id,'requestId',request_id,'campaignId',campaign_id,'creativeId',creative_id,'type',event_type,'valueFen',value_fen,'occurredAt','2026-09-01T00:00:00Z'),
 CASE WHEN MOD(CAST(SUBSTRING(request_id,5) AS UNSIGNED),100)<90 THEN 'PUBLISHED'
 WHEN MOD(CAST(SUBSTRING(request_id,5) AS UNSIGNED),100)<95 THEN 'SETTLING'
 WHEN MOD(CAST(SUBSTRING(request_id,5) AS UNSIGNED),100)<98 THEN 'PENDING'
 WHEN MOD(CAST(SUBSTRING(request_id,5) AS UNSIGNED),100)=98 THEN 'PROCESSING' ELSE 'RECONCILE' END,
 0,occurred_at,created_at FROM event_receipts ORDER BY event_id`); err != nil {
		return
	}
	d.SeedSeconds = time.Since(started).Seconds()
	started = time.Now()
	if _, err = c.ExecContext(ctx, "ALTER TABLE event_outbox ADD INDEX idx_outbox_request_status (aggregate_key,status) INVISIBLE"); err != nil {
		return
	}
	d.IndexBuildSeconds = time.Since(started).Seconds()
	if _, err = readRows(ctx, c, "ANALYZE TABLE event_outbox", nil); err != nil {
		return
	}
	if err = c.QueryRowContext(ctx, `SELECT stat_value * @@innodb_page_size FROM mysql.innodb_index_stats WHERE database_name=? AND table_name='event_outbox' AND index_name='idx_outbox_request_status' AND stat_name='size'`, d.Schema).Scan(&d.IndexBytes); err != nil {
		return
	}
	var rows *sql.Rows
	rows, err = c.QueryContext(ctx, "SELECT status,COUNT(*) FROM event_outbox GROUP BY status")
	if err != nil {
		return
	}
	for rows.Next() {
		var s string
		var count int
		if err = rows.Scan(&s, &count); err != nil {
			rows.Close()
			return
		}
		d.Counts[s] = count
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return
	}
	total := 0
	for _, count := range d.Counts {
		total += count
	}
	if total != n {
		err = fmt.Errorf("seed count %d != %d", total, n)
		return
	}
	for round := 0; round < rounds; round++ {
		modes := []string{"invisible", "visible"}
		if round%2 == 1 {
			modes[0], modes[1] = modes[1], modes[0]
		}
		for _, mode := range modes {
			if _, err = c.ExecContext(ctx, "ALTER TABLE event_outbox ALTER INDEX idx_outbox_request_status "+strings.ToUpper(mode)); err != nil {
				return
			}
			targets := targetRequests(n, round, reps+warmups)
			for j := 0; j < warmups; j++ {
				for _, op := range []string{"select", "update"} {
					if _, err = sample(ctx, c, thread, op, targets[j], round, mode); err != nil {
						return
					}
				}
			}
			if _, ok := d.Plans[mode]; !ok {
				d.Plans[mode] = map[string]any{}
				for name, statement := range map[string]string{"selectAnalyze": "EXPLAIN ANALYZE " + selectSQL, "selectJSON": "EXPLAIN FORMAT=JSON " + selectSQL, "updateJSON": "EXPLAIN FORMAT=JSON " + updateSQL} {
					var data []map[string]string
					data, err = readRows(ctx, c, statement, []any{targets[0]})
					if err != nil {
						return
					}
					d.Plans[mode][name] = data
				}
				var indexes []map[string]string
				indexes, err = readRows(ctx, c, "SHOW INDEX FROM event_outbox", nil)
				if err != nil {
					return
				}
				d.Plans[mode]["indexes"] = indexes
			}
			blockStart := time.Now()
			for j := warmups; j < len(targets); j++ {
				for _, op := range []string{"select", "update"} {
					var m measurement
					m, err = sample(ctx, c, thread, op, targets[j], round, mode)
					if err != nil {
						return
					}
					d.Measurements = append(d.Measurements, m)
				}
			}
			fmt.Printf("rows=%d round=%d mode=%s: %d SELECT+UPDATE pairs in %.3fs\n", n, round+1, mode, reps, time.Since(blockStart).Seconds())
		}
	}
	for _, mode := range []string{"invisible", "visible"} {
		for _, op := range []string{"select", "update"} {
			var client, server, examined []float64
			for _, m := range d.Measurements {
				if m.Mode == mode && m.Operation == op {
					client = append(client, m.ClientMS)
					server = append(server, m.ServerMS)
					examined = append(examined, float64(m.RowsExamined))
				}
			}
			s := summary{Mode: mode, Operation: op, Samples: len(client), ClientMS: stats(client), ServerMS: stats(server), Examined: stats(examined)}
			d.Summary = append(d.Summary, s)
			fmt.Printf("SUMMARY rows=%d %s %s n=%d server median=%.3fms p95=%.3fms examined=%.0f\n", n, mode, op, s.Samples, s.ServerMS.Median, s.ServerMS.P95, s.Examined.Median)
		}
	}
	// All UPDATE samples rolled back: verify seeded status distribution unchanged.
	for status, expected := range d.Counts {
		var count int
		if err = c.QueryRowContext(ctx, "SELECT COUNT(*) FROM event_outbox WHERE status=?", status).Scan(&count); err != nil {
			return
		}
		if count != expected {
			err = fmt.Errorf("dataset changed for %s: %d != %d", status, count, expected)
			return
		}
	}
	if !strings.HasPrefix(d.Schema, "adflow_idxbench_") {
		err = fmt.Errorf("refusing cleanup outside generated benchmark schema")
		return
	}
	if _, err = c.ExecContext(ctx, "DROP DATABASE `"+d.Schema+"`"); err != nil {
		return
	}
	d.Cleanup = true
	return
}
func targetRequests(n, round, count int) []string {
	groups := n / 300
	out := make([]string, count)
	for i := range out {
		id := ((i*7919+round*3571)%groups)*100 + 90 + (i % 5)
		out[i] = fmt.Sprintf("req-%012d", id)
	}
	return out
}
func sample(ctx context.Context, c *sql.Conn, thread int64, op, request string, round int, mode string) (m measurement, err error) {
	m = measurement{Round: round + 1, Mode: mode, Operation: op, Request: request}
	if op == "update" {
		if _, err = c.ExecContext(ctx, "START TRANSACTION"); err != nil {
			return
		}
		defer func() {
			_, e := c.ExecContext(context.WithoutCancel(ctx), "ROLLBACK")
			if err == nil {
				err = e
			}
		}()
	}
	start := time.Now()
	if op == "select" {
		var rows *sql.Rows
		rows, err = c.QueryContext(ctx, selectSQL, request)
		if err != nil {
			return
		}
		for rows.Next() {
			var id, status string
			var attempts int
			var next time.Time
			var last sql.NullString
			if err = rows.Scan(&id, &status, &attempts, &next, &last); err != nil {
				rows.Close()
				return
			}
			m.Count++
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return
		}
	} else {
		var result sql.Result
		result, err = c.ExecContext(ctx, updateSQL, request)
		if err != nil {
			return
		}
		var count int64
		count, err = result.RowsAffected()
		if err != nil {
			return
		}
		m.Count = int(count)
	}
	m.ClientMS = float64(time.Since(start).Nanoseconds()) / 1e6
	if m.Count != 3 {
		err = fmt.Errorf("%s expected 3 rows for %s, got %d", op, request, m.Count)
		return
	}
	var timer, lock int64
	var sqlText string
	err = c.QueryRowContext(ctx, `SELECT TIMER_WAIT,LOCK_TIME,ROWS_EXAMINED,ROWS_AFFECTED,ROWS_SENT,SQL_TEXT
 FROM performance_schema.events_statements_history WHERE THREAD_ID=? AND SQL_TEXT LIKE ? ORDER BY EVENT_ID DESC LIMIT 1`, thread, strings.ToUpper(op)+"%event_outbox%").Scan(&timer, &lock, &m.RowsExamined, &m.RowsAffected, &m.RowsSent, &sqlText)
	if err != nil {
		return
	}
	if !strings.Contains(sqlText, request) {
		err = fmt.Errorf("statement telemetry does not match target request")
		return
	}
	m.ServerMS = float64(timer) / 1e9
	m.LockMS = float64(lock) / 1e9
	return
}
func readRows(ctx context.Context, c *sql.Conn, query string, args []any) ([]map[string]string, error) {
	rows, err := c.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var result []map[string]string
	for rows.Next() {
		values := make([]sql.NullString, len(cols))
		ptr := make([]any, len(cols))
		for i := range ptr {
			ptr[i] = &values[i]
		}
		if err = rows.Scan(ptr...); err != nil {
			return nil, err
		}
		row := map[string]string{}
		for i, key := range cols {
			if values[i].Valid {
				row[key] = values[i].String
			} else {
				row[key] = "NULL"
			}
		}
		result = append(result, row)
	}
	return result, rows.Err()
}
func stats(values []float64) distribution {
	sort.Float64s(values)
	n := len(values)
	median := values[n/2]
	if n%2 == 0 {
		median = (values[n/2-1] + median) / 2
	}
	return distribution{Median: median, P95: values[(95*n+99)/100-1], Min: values[0], Max: values[n-1]}
}
