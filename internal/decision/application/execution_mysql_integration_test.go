//go:build integration

package application

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
	memoryadapter "github.com/umi3730/adflow/internal/decision/adapter/memory"
	mysqladapter "github.com/umi3730/adflow/internal/decision/adapter/mysql"
	redisadapter "github.com/umi3730/adflow/internal/decision/adapter/redis"
	"github.com/umi3730/adflow/internal/decision/domain"
	"github.com/umi3730/adflow/internal/platform/migrate"
)

func TestExecutionCoordinationWithRealMySQLAndRedis(t *testing.T) {
	dsn := os.Getenv("ADFLOW_IDEMPOTENCY_MYSQL_DSN")
	redisAddr := os.Getenv("ADFLOW_IDEMPOTENCY_REDIS_ADDR")
	if dsn == "" || redisAddr == "" {
		t.Skip("opt-in: provide isolated-test MySQL credentials and local Redis address")
	}
	cfg, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatal("invalid test DSN")
	}
	local := func(addr string) bool {
		host, _, err := net.SplitHostPort(addr)
		return err == nil && (host == "localhost" || host == "127.0.0.1" || host == "::1")
	}
	if cfg.Net != "tcp" || !local(cfg.Addr) || !local(redisAddr) {
		t.Skip("test accepts loopback dependencies only")
	}
	var random [6]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	suffix := hex.EncodeToString(random[:])
	databaseName := "adflow_idempotency_test_" + suffix
	cfg.DBName = ""
	cfg.ParseTime = true
	admin, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE `"+databaseName+"`"); err != nil {
		t.Skipf("cannot create a separate disposable database: %v", err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if _, err := admin.ExecContext(cleanup, "DROP DATABASE `"+databaseName+"`"); err != nil {
			t.Errorf("remove owned test database: %v", err)
		}
	}()
	cfg.DBName = databaseName
	firstDB, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer firstDB.Close()
	secondDB, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer secondDB.Close()
	firstDB.SetMaxOpenConns(12)
	secondDB.SetMaxOpenConns(12)
	runner, err := migrate.NewRunner(firstDB, filepath.Join("..", "..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Up(ctx); err != nil {
		t.Fatal(err)
	}
	prefix := "adflow:idem-test:" + suffix
	firstRedis := redis.NewClient(&redis.Options{Addr: redisAddr, Password: os.Getenv("ADFLOW_IDEMPOTENCY_REDIS_PASSWORD")})
	defer firstRedis.Close()
	secondRedis := redis.NewClient(&redis.Options{Addr: redisAddr, Password: os.Getenv("ADFLOW_IDEMPOTENCY_REDIS_PASSWORD")})
	defer secondRedis.Close()
	if err := firstRedis.Ping(ctx).Err(); err != nil {
		t.Skipf("local Redis unavailable: %v", err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		var keys []string
		var cursor uint64
		for {
			batch, next, err := firstRedis.Scan(cleanup, cursor, prefix+":*", 100).Result()
			if err != nil {
				t.Error(err)
				return
			}
			keys = append(keys, batch...)
			cursor = next
			if cursor == 0 {
				break
			}
		}
		for _, key := range keys {
			if !strings.HasPrefix(key, prefix+":") {
				t.Fatal("test cleanup namespace mismatch")
			}
		}
		if len(keys) > 0 {
			if err := firstRedis.Del(cleanup, keys...).Err(); err != nil {
				t.Error(err)
			}
		}
	}()
	profiles := memoryadapter.NewRuntime()
	_ = profiles.PutProfile(ctx, domain.NewProfile("user", []string{"auction"}, nil))
	candidate := bidCandidate("winner", "studio", 5)
	candidate.DailyBudgetFen = 5
	candidate.FrequencyLimit = 1
	firstStore, secondStore := mysqladapter.NewStore(firstDB), mysqladapter.NewStore(secondDB)
	firstGate, secondGate := redisadapter.NewReservations(firstRedis, prefix), redisadapter.NewReservations(secondRedis, prefix)
	engines := []*Service{NewService(candidateProvider{[]domain.Candidate{candidate}}, profiles, firstGate, firstGate, firstStore), NewService(candidateProvider{[]domain.Candidate{candidate}}, profiles, secondGate, secondGate, secondStore)}
	request := domain.Request{RequestID: "shared-request", UserID: "user", SlotID: "slot"}
	type response struct {
		result domain.Result
		err    error
	}
	responses := make(chan response, 24)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for i := 0; i < 24; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			<-start
			r, err := engines[i%2].Decide(ctx, request)
			responses <- response{r, err}
		}(i)
	}
	close(start)
	workers.Wait()
	close(responses)
	successes, busy := 0, 0
	var canonical domain.Result
	for r := range responses {
		if errors.Is(r.err, domain.ErrDecisionInProgress) {
			busy++
			continue
		}
		if r.err != nil {
			t.Fatal(r.err)
		}
		if !r.result.Matched {
			t.Fatalf("duplicate produced No-Ad: %+v", r.result)
		}
		if successes > 0 && r.result != canonical {
			t.Fatal("shared stores returned different decisions")
		}
		canonical = r.result
		successes++
	}
	if successes == 0 {
		t.Fatal("no request completed")
	}
	for _, engine := range engines {
		again, err := engine.Decide(ctx, request)
		if err != nil || again != canonical {
			t.Fatalf("replay=%+v err=%v", again, err)
		}
	}
	changed := request
	changed.UserID = "other"
	if _, err := engines[1].Decide(ctx, changed); !errors.Is(err, domain.ErrRequestConflict) {
		t.Fatalf("parameter binding: %v", err)
	}
	now := time.Now().UTC()
	if _, allowed, err := secondGate.ReserveBudget(ctx, "winner", 5, 5, "budget-probe", now, time.Second); err != nil || allowed {
		t.Fatalf("winner budget lost: allowed=%v err=%v", allowed, err)
	}
	if _, allowed, err := secondGate.ReserveFrequency(ctx, "user", "winner", "frequency-probe", 1, now, time.Second); err != nil || allowed {
		t.Fatalf("winner frequency lost: allowed=%v err=%v", allowed, err)
	}
	if err := firstGate.ConfirmBudget(ctx, canonical.ReservationToken, now); err != nil {
		t.Fatal(err)
	}
	if err := secondGate.ConfirmBudget(ctx, canonical.ReservationToken, now); err != nil {
		t.Fatal(err)
	}
	spent, err := firstGate.DebugBudgetSpent(ctx, "winner", now)
	if err != nil || spent != 5 {
		t.Fatalf("spent=%d err=%v", spent, err)
	}
	report := map[string]any{"at": time.Now().UTC(), "services": 2, "concurrentRequests": 24, "successfulIdenticalResults": successes, "boundedInProgressResponses": busy, "bindingConflictRejected": true, "winnerBudgetRetained": true, "winnerFrequencyRetained": true, "duplicateConfirmationSpentFen": spent, "dedicatedDatabase": true, "isolatedRedisPrefix": true}
	encoded, _ := json.Marshal(report)
	t.Log(string(encoded))
	if path := os.Getenv("ADFLOW_IDEMPOTENCY_REPORT"); path != "" {
		if err := os.WriteFile(path, append(encoded, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
