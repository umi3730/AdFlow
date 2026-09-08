//go:build integration

package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"github.com/redis/go-redis/v9"
	decisionmysql "github.com/zhanghaiyang/adflow/internal/decision/adapter/mysql"
	decisionredis "github.com/zhanghaiyang/adflow/internal/decision/adapter/redis"
	decisiondomain "github.com/zhanghaiyang/adflow/internal/decision/domain"
	eventmysql "github.com/zhanghaiyang/adflow/internal/event/adapter/mysql"
	"github.com/zhanghaiyang/adflow/internal/event/domain"
	"github.com/zhanghaiyang/adflow/internal/platform/database"
	"os"
	"testing"
	"time"
)

// The supplied dedicated DB must already have the migrations. The application
// clock advances without sleeping; Redis settlement and SQL writes are real.
func TestAttributionWithRealReceiptsAndDelayedConsumer(t *testing.T) {
	dsn, address := os.Getenv("ADFLOW_IT_MYSQL_DSN"), os.Getenv("ADFLOW_IT_REDIS_ADDR")
	if dsn == "" || address == "" {
		t.Skip("set isolated ADFLOW_IT_MYSQL_DSN and ADFLOW_IT_REDIS_ADDR")
	}
	db, err := database.OpenMySQL(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	client := redis.NewClient(&redis.Options{Addr: address, Protocol: 2, DisableIdentity: true})
	defer client.Close()
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	id := hex.EncodeToString(nonce[:])
	prefix := "adflow:it:attribution:" + id
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var cursor uint64
		for {
			keys, next, err := client.Scan(ctx, cursor, prefix+":*", 100).Result()
			if err != nil {
				t.Error(err)
				break
			}
			if len(keys) > 0 {
				if err := client.Del(ctx, keys...).Err(); err != nil {
					t.Error(err)
				}
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
		for _, query := range []string{"DELETE FROM processed_events WHERE request_id = ?", "DELETE FROM event_outbox WHERE aggregate_key = ?", "DELETE FROM event_receipts WHERE request_id = ?", "DELETE FROM event_settlements WHERE request_id = ?", "DELETE FROM decisions WHERE request_id = ?"} {
			if _, err := db.ExecContext(ctx, query, id); err != nil {
				t.Error(err)
			}
		}
		if _, err := db.ExecContext(ctx, "DELETE FROM campaign_metrics WHERE campaign_id = ?", id); err != nil {
			t.Error(err)
		}
	}()
	start := time.Now().UTC()
	clock := start
	reservations := decisionredis.NewReservations(client, prefix)
	if _, ok, err := reservations.ReserveFrequency(t.Context(), id, id, id, 3, start, 30*time.Second); err != nil || !ok {
		t.Fatal(err)
	}
	if _, ok, err := reservations.ReserveBudget(t.Context(), id, 100, 5, id, start, 30*time.Second); err != nil || !ok {
		t.Fatal(err)
	}
	decisions := decisionmysql.NewStore(db)
	decision := decisiondomain.Result{RequestID: id, UserID: id, Matched: true, CampaignID: id, CreativeID: id, ReservationToken: id, ExpiresAt: start.Add(30 * time.Second), Pricing: decisiondomain.Pricing{Mode: "fixed", PriceFen: 5}}
	if err := decisions.SaveDecision(t.Context(), decision); err != nil {
		t.Fatal(err)
	}
	outbox := eventmysql.NewOutbox(db, "attribution-"+id)
	metrics := eventmysql.NewStore(db)
	processor := NewService(metrics, decisions, nil)
	ingress := NewAsyncService(processor, decisions, outbox)
	ingress.now = func() time.Time { return clock }
	impression := domain.Event{EventID: id + "-impression", RequestID: id, CampaignID: id, CreativeID: id, Type: domain.Impression, OccurredAt: start}
	if _, err := ingress.Record(t.Context(), impression); err != nil {
		t.Fatal(err)
	}
	if count, err := NewSettlementWorker(outbox, reservations, nil).RunOnce(t.Context()); err != nil || count != 1 {
		t.Fatalf("settlement count=%d err=%v", count, err)
	}
	if err := RecordSettledBatch(outbox, processor)(t.Context(), []domain.Event{impression}); err != nil {
		t.Fatal(err)
	}
	clock = start.Add(time.Hour)
	callback := impression
	callback.EventID = id + "-click"
	callback.Type = domain.Click
	callback.OccurredAt = clock
	if _, err := ingress.Record(t.Context(), callback); err != nil {
		t.Fatal(err)
	}
	// Consumption can occur after the attribution window, provided ingress
	// durably accepted the unchanged event within the window.
	processor.now = func() time.Time { return start.Add(8 * 24 * time.Hour) }
	if err := RecordSettledBatch(outbox, processor)(t.Context(), []domain.Event{callback}); err != nil {
		t.Fatal(err)
	}
	clock = start.Add(domain.DefaultAttributionWindow + time.Minute)
	if created, err := ingress.Record(t.Context(), callback); err != nil || created {
		t.Fatalf("late duplicate created=%v err=%v", created, err)
	}
	callback.EventID = id + "-too-late"
	callback.Type = domain.Conversion
	callback.OccurredAt = clock
	if _, err := ingress.Record(t.Context(), callback); !errors.Is(err, domain.ErrAttributionExpired) {
		t.Fatal(err)
	}
	result, err := metrics.Metrics(t.Context(), id)
	if err != nil || result.Impressions != 1 || result.Clicks != 1 {
		t.Fatalf("metrics=%+v err=%v", result, err)
	}
	spent, err := reservations.DebugBudgetSpent(t.Context(), id, start)
	if err != nil || spent != 5 {
		t.Fatalf("spent=%d err=%v", spent, err)
	}
}
