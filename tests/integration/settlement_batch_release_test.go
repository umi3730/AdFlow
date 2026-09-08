//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	decisionredis "github.com/zhanghaiyang/adflow/internal/decision/adapter/redis"
	decisiondomain "github.com/zhanghaiyang/adflow/internal/decision/domain"
	eventmysql "github.com/zhanghaiyang/adflow/internal/event/adapter/mysql"
	eventapp "github.com/zhanghaiyang/adflow/internal/event/application"
	eventdomain "github.com/zhanghaiyang/adflow/internal/event/domain"
)

type failOneCompletion struct {
	*eventmysql.Outbox
	failed bool
	claims []eventdomain.SettlementEntry
}

func (q *failOneCompletion) ClaimSettlements(ctx context.Context, limit int, lease time.Duration) ([]eventdomain.SettlementEntry, error) {
	entries, err := q.Outbox.ClaimSettlements(ctx, limit, lease)
	q.claims = append([]eventdomain.SettlementEntry(nil), entries...)
	return entries, err
}
func (q *failOneCompletion) CompleteSettlement(ctx context.Context, e eventdomain.SettlementEntry) error {
	if !q.failed {
		q.failed = true
		return errors.New("injected completion persistence failure")
	}
	return q.Outbox.CompleteSettlement(ctx, e)
}

func TestSettlementBatchReleaseAndFencingWithRealStores(t *testing.T) {
	db := integrationMySQL(t)
	client := redis.NewClient(&redis.Options{Addr: integrationEnv(t, "ADFLOW_IT_REDIS_ADDR"), Protocol: 2, DisableIdentity: true})
	t.Cleanup(func() { _ = client.Close() })
	id := newID(t)
	prefix := "batch-recovery-it:" + id
	r := decisionredis.NewReservations(client, prefix)
	outbox := eventmysql.NewOutbox(db, "first")
	now := time.Now().UTC()
	var decisions []decisiondomain.Result
	t.Cleanup(func() {
		if err := deletePrefix(context.Background(), client, prefix); err != nil {
			t.Error(err)
		}
		for _, d := range decisions {
			_, _ = db.Exec("DELETE FROM event_outbox WHERE aggregate_key=?", d.RequestID)
			_, _ = db.Exec("DELETE FROM event_receipts WHERE request_id=?", d.RequestID)
			_, _ = db.Exec("DELETE FROM event_settlements WHERE request_id=?", d.RequestID)
		}
	})
	for i := 0; i < 3; i++ {
		d := decisiondomain.Result{RequestID: fmt.Sprintf("batch-%s-%d", id, i), UserID: fmt.Sprintf("user-%d", i), CampaignID: id, CreativeID: id, Matched: true, ReservationToken: fmt.Sprintf("token-%d", i), ExpiresAt: now.Add(30 * time.Second), Pricing: decisiondomain.Pricing{PriceFen: 5}}
		decisions = append(decisions, d)
		if _, ok, err := r.ReserveFrequency(t.Context(), d.UserID, id, d.ReservationToken, 1, now, 30*time.Second); err != nil || !ok {
			t.Fatal(err)
		}
		if _, ok, err := r.ReserveBudget(t.Context(), id, 15, 5, d.ReservationToken, now, 30*time.Second); err != nil || !ok {
			t.Fatal(err)
		}
		event := eventdomain.Event{EventID: "event-" + d.RequestID, RequestID: d.RequestID, CampaignID: id, CreativeID: id, Type: eventdomain.Impression, OccurredAt: now}
		if created, err := outbox.EnqueueForSettlement(t.Context(), event, d, now); err != nil || !created {
			t.Fatalf("enqueue=%v %v", created, err)
		}
	}
	q := &failOneCompletion{Outbox: outbox}
	if _, err := eventapp.NewSettlementWorker(q, r, nil).RunOnce(t.Context()); err == nil {
		t.Fatal("failure not injected")
	}
	var pending int
	if err := db.QueryRow("SELECT COUNT(*) FROM event_settlements WHERE request_id LIKE ? AND status='PENDING' AND locked_by IS NULL", "batch-"+id+"-%").Scan(&pending); err != nil || pending != 3 {
		t.Fatalf("stranded=%d %v", pending, err)
	}
	second := eventmysql.NewOutbox(db, "second")
	claims, err := second.ClaimSettlements(t.Context(), 4, 15*time.Second)
	if err != nil || len(claims) != 3 {
		t.Fatalf("immediate claims=%d %v", len(claims), err)
	}
	if err := outbox.ReleaseSettlements(t.Context(), q.claims); err != nil {
		t.Fatal(err)
	}
	var owned int
	if err := db.QueryRow("SELECT COUNT(*) FROM event_settlements WHERE locked_by=?", claims[0].Owner).Scan(&owned); err != nil || owned != 3 {
		t.Fatalf("stale release clobbered new owner: %d %v", owned, err)
	}
	if err := second.ReleaseSettlements(t.Context(), claims); err != nil {
		t.Fatal(err)
	}
	if n, err := eventapp.NewSettlementWorker(second, r, nil).RunOnce(t.Context()); err != nil || n != 3 {
		t.Fatalf("recovery=%d %v", n, err)
	}
	if err := outbox.ReleaseSettlements(t.Context(), q.claims); err != nil {
		t.Fatal(err)
	}
	var settled int
	if err := db.QueryRow("SELECT COUNT(*) FROM event_settlements WHERE request_id LIKE ? AND status='SETTLED'", "batch-"+id+"-%").Scan(&settled); err != nil || settled != 3 {
		t.Fatalf("settled=%d %v", settled, err)
	}
	if spent, err := r.DebugBudgetSpent(t.Context(), id, now); err != nil || spent != 15 {
		t.Fatalf("duplicate charge: %d %v", spent, err)
	}
}
