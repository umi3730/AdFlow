//go:build integration

package integration

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	campaignmysql "github.com/umi3730/adflow/internal/campaign/adapter/mysql"
	campaigndomain "github.com/umi3730/adflow/internal/campaign/domain"
	decisionmysql "github.com/umi3730/adflow/internal/decision/adapter/mysql"
	decisionprofilecache "github.com/umi3730/adflow/internal/decision/adapter/profilecache"
	decisionredis "github.com/umi3730/adflow/internal/decision/adapter/redis"
	decisiondomain "github.com/umi3730/adflow/internal/decision/domain"
	eventmysql "github.com/umi3730/adflow/internal/event/adapter/mysql"
	eventdomain "github.com/umi3730/adflow/internal/event/domain"
	"github.com/umi3730/adflow/internal/platform/database"
	"github.com/umi3730/adflow/internal/platform/migrate"
)

func TestMySQLCampaignOptimisticConcurrency(t *testing.T) {
	db := integrationMySQL(t)
	repository := campaignmysql.NewRepository(db)
	id := newID(t)
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM campaigns WHERE id = ?`, id) })
	name, _ := campaigndomain.NewName("Integration Campaign")
	slot, _ := campaigndomain.NewSlotID("integration-banner")
	period, _ := campaigndomain.NewDeliveryPeriod(time.Now().UTC().Add(-time.Minute), time.Now().UTC().Add(time.Hour))
	campaign := campaigndomain.NewCampaign(id, name, slot, period)
	if err := repository.Create(t.Context(), campaign); err != nil {
		t.Fatal(err)
	}
	first, err := repository.FindByID(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	second, err := repository.FindByID(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	updatedName, _ := campaigndomain.NewName("First Writer")
	if err := first.UpdateDraft(updatedName, slot, period); err != nil {
		t.Fatal(err)
	}
	if err := repository.Save(t.Context(), first, 1); err != nil {
		t.Fatal(err)
	}
	staleName, _ := campaigndomain.NewName("Stale Writer")
	if err := second.UpdateDraft(staleName, slot, period); err != nil {
		t.Fatal(err)
	}
	if err := repository.Save(t.Context(), second, 1); !errors.Is(err, campaigndomain.ErrConcurrentMutation) {
		t.Fatalf("stale save error=%v", err)
	}
	persisted, err := repository.FindByID(t.Context(), id)
	if err != nil || string(persisted.Name()) != "First Writer" || persisted.Revision() != 2 {
		t.Fatalf("persisted=%+v err=%v", persisted, err)
	}
}

func TestMySQLOutboxLeaseAllowsOneRelayOwner(t *testing.T) {
	db := integrationMySQL(t)
	eventID := "evt-" + newID(t)
	requestID := "req-" + newID(t)
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM event_outbox WHERE event_id = ?`, eventID)
		_, _ = db.Exec(`DELETE FROM event_receipts WHERE event_id = ?`, eventID)
		_, _ = db.Exec(`DELETE FROM event_settlements WHERE request_id = ?`, requestID)
	})
	event := eventdomain.Event{EventID: eventID, RequestID: requestID, CampaignID: newID(t), CreativeID: newID(t), Type: eventdomain.Impression, OccurredAt: time.Now().UTC()}
	first := eventmysql.NewOutbox(db, "integration-relay-1")
	second := eventmysql.NewOutbox(db, "integration-relay-2")
	created, err := first.EnqueueForSettlement(t.Context(), event, decisiondomain.Result{RequestID: requestID}, event.OccurredAt)
	if err != nil || !created {
		t.Fatalf("enqueue created=%v err=%v", created, err)
	}
	// This test isolates relay leasing. Settle its owned fixture through the
	// queue contract; reservation settlement itself is covered separately.
	settlements, err := first.ClaimSettlements(t.Context(), 1, time.Minute)
	if err != nil || len(settlements) != 1 {
		t.Fatalf("settlement claims=%v err=%v", settlements, err)
	}
	if err := first.CompleteSettlement(t.Context(), settlements[0]); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	type claim struct {
		worker  string
		outbox  *eventmysql.Outbox
		entries []eventdomain.OutboxEntry
		err     error
	}
	claims := make(chan claim, 2)
	var wait sync.WaitGroup
	for worker, outbox := range map[string]*eventmysql.Outbox{"integration-relay-1": first, "integration-relay-2": second} {
		wait.Add(1)
		go func(worker string, outbox *eventmysql.Outbox) {
			defer wait.Done()
			entries, callErr := outbox.ClaimBatch(context.Background(), 1, now, 5*time.Second)
			claims <- claim{worker: worker, outbox: outbox, entries: entries, err: callErr}
		}(worker, outbox)
	}
	wait.Wait()
	close(claims)
	owners := 0
	for result := range claims {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if len(result.entries) == 1 {
			owners++
			if result.entries[0].Event.EventID != eventID {
				t.Fatalf("worker %s claimed unexpected event", result.worker)
			}
			if err := result.outbox.MarkPublished(t.Context(), eventID, time.Now().UTC()); err != nil {
				t.Fatal(err)
			}
		}
	}
	if owners != 1 {
		t.Fatalf("relay owners=%d, want 1", owners)
	}
	stats, err := first.Stats(t.Context())
	if err != nil || stats.Published < 1 {
		t.Fatalf("stats=%+v err=%v", stats, err)
	}
}

func TestMySQLEventBatchIsAtomicAndIdempotent(t *testing.T) {
	db := integrationMySQL(t)
	store := eventmysql.NewStore(db)
	campaignID := newID(t)
	requestID := "req-" + newID(t)
	first := eventdomain.Event{
		EventID: "evt-" + newID(t), RequestID: requestID, CampaignID: campaignID,
		CreativeID: newID(t), Type: eventdomain.Impression, OccurredAt: time.Now().UTC(),
	}
	second := eventdomain.Event{
		EventID: "evt-" + newID(t), RequestID: requestID, CampaignID: campaignID,
		CreativeID: first.CreativeID, Type: eventdomain.Click, OccurredAt: first.OccurredAt.Add(time.Millisecond),
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM campaign_metrics WHERE campaign_id = ?`, campaignID)
		_, _ = db.Exec(`DELETE FROM processed_events WHERE event_id IN (?, ?)`, first.EventID, second.EventID)
	})
	created, err := store.RecordBatch(t.Context(), []eventdomain.Event{first, second, first})
	if err != nil || len(created) != 3 || !created[0] || !created[1] || created[2] {
		t.Fatalf("created=%v err=%v", created, err)
	}
	metrics, err := store.Metrics(t.Context(), campaignID)
	if err != nil || metrics.Impressions != 1 || metrics.Clicks != 1 {
		t.Fatalf("metrics=%+v err=%v", metrics, err)
	}
}

func TestRedisSlidingWindowAndBudgetReservationAreAtomic(t *testing.T) {
	address := integrationEnv(t, "ADFLOW_IT_REDIS_ADDR")
	client := redis.NewClient(&redis.Options{Addr: address, Protocol: 2, DisableIdentity: true})
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Ping(t.Context()).Err(); err != nil {
		t.Fatal(err)
	}
	prefix := "adflow:it:" + newID(t)
	t.Cleanup(func() {
		if err := deletePrefix(context.Background(), client, prefix); err != nil {
			t.Errorf("clean Redis test keys: %v", err)
		}
	})
	// This test isolates atomic admission under concurrency. Expiry behavior has a
	// deterministic unit test, so use a long window here to avoid host/WSL clock
	// synchronization changing the time bucket while goroutines are in flight.
	limiter, err := decisionredis.NewSlidingWindowLimiter(client, prefix+":limit", 10, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	var allowed atomic.Int64
	var wait sync.WaitGroup
	for index := 0; index < 50; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			ok, callErr := limiter.Allow(context.Background(), fmt.Sprintf("%s-limit-%d", prefix, index))
			if callErr != nil {
				t.Errorf("limiter error=%v", callErr)
			} else if ok {
				allowed.Add(1)
			}
		}(index)
	}
	wait.Wait()
	if allowed.Load() != 10 {
		t.Fatalf("sliding-window allowed=%d, want 10", allowed.Load())
	}
	reservations := decisionredis.NewReservations(client, prefix)
	allowed.Store(0)
	now := time.Now().UTC()
	for index := 0; index < 50; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, ok, callErr := reservations.ReserveBudget(context.Background(), "campaign", 1000, 100, fmt.Sprintf("%s-budget-%d", prefix, index), now, time.Second)
			if callErr != nil {
				t.Errorf("budget reservation error=%v", callErr)
			} else if ok {
				allowed.Add(1)
			}
		}(index)
	}
	wait.Wait()
	if allowed.Load() != 10 {
		t.Fatalf("budget reservations=%d, want 10", allowed.Load())
	}
	confirmationCampaign := "confirm-" + newID(t)
	confirmationToken := "confirm-" + newID(t)
	_, ok, err := reservations.ReserveBudget(t.Context(), confirmationCampaign, 1000, 100, confirmationToken, now, time.Second)
	if err != nil || !ok {
		t.Fatalf("confirmation reservation allowed=%v err=%v", ok, err)
	}
	if err := reservations.ConfirmBudget(t.Context(), confirmationToken, now); err != nil {
		t.Fatal(err)
	}
	spent, err := reservations.DebugBudgetSpent(t.Context(), confirmationCampaign, now)
	if err != nil || spent != 100 {
		t.Fatalf("confirmed budget spent=%d err=%v", spent, err)
	}
}

func TestRedisMySQLProfileCacheAsideReturnsWrittenProfile(t *testing.T) {
	db := integrationMySQL(t)
	address := integrationEnv(t, "ADFLOW_IT_REDIS_ADDR")
	client := redis.NewClient(&redis.Options{Addr: address, Protocol: 2, DisableIdentity: true})
	t.Cleanup(func() { _ = client.Close() })
	userID := "cache-user-" + newID(t)
	prefix := "adflow:it:profile:" + newID(t)
	t.Cleanup(func() {
		if err := deletePrefix(context.Background(), client, prefix); err != nil {
			t.Errorf("clean Redis test keys: %v", err)
		}
		_, _ = db.Exec(`DELETE FROM user_profiles WHERE user_id = ?`, userID)
	})
	store, err := decisionprofilecache.New(decisionmysql.NewProfileStore(db), client, prefix, time.Minute, 5*time.Second, 20*time.Millisecond, nil)
	if err != nil {
		t.Fatal(err)
	}
	profile := decisiondomain.NewProfile(userID, []string{"anime"}, map[string]string{"device": "ios"})
	if err := store.PutProfile(t.Context(), profile); err != nil {
		t.Fatal(err)
	}
	// Remove the source row to prove the following read is served by Redis.
	if _, err := db.ExecContext(t.Context(), `DELETE FROM user_profiles WHERE user_id = ?`, userID); err != nil {
		t.Fatal(err)
	}
	cached, err := store.FindProfile(t.Context(), userID)
	if err != nil || cached.Fields["device"] != "ios" {
		t.Fatalf("cached=%+v err=%v", cached, err)
	}
}

func integrationMySQL(t *testing.T) *sql.DB {
	t.Helper()
	dsn := integrationEnv(t, "ADFLOW_IT_MYSQL_DSN")
	db, err := database.OpenMySQL(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	directory, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	runner, err := migrate.NewRunner(db, directory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Up(ctx); err != nil {
		t.Fatal(err)
	}
	return db
}

func integrationEnv(t *testing.T, key string) string {
	t.Helper()
	value := os.Getenv(key)
	if value == "" {
		t.Skipf("set %s to run infrastructure integration tests", key)
	}
	return value
}

func newID(t testing.TB) string {
	t.Helper()
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(value[:])
}

func deletePrefix(ctx context.Context, client *redis.Client, prefix string) error {
	if prefix == "" {
		return errors.New("refusing empty Redis cleanup prefix")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var cursor uint64
	for {
		keys, next, err := client.Scan(ctx, cursor, prefix+"*", 100).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err := client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		cursor = next
		if cursor == 0 {
			return nil
		}
	}
}
