package application

import (
	"context"
	"errors"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	decisionredis "github.com/zhanghaiyang/adflow/internal/decision/adapter/redis"
	decisiondomain "github.com/zhanghaiyang/adflow/internal/decision/domain"
	eventmemory "github.com/zhanghaiyang/adflow/internal/event/adapter/memory"
	"github.com/zhanghaiyang/adflow/internal/event/domain"
	"testing"
	"time"
)

type settlementTestQueue struct {
	entry         domain.SettlementEntry
	ready, review bool
	completeError error
	failures      int
}

func (q *settlementTestQueue) ClaimSettlements(context.Context, int, time.Duration) ([]domain.SettlementEntry, error) {
	if q.ready || q.review {
		return nil, nil
	}
	return []domain.SettlementEntry{q.entry}, nil
}
func (q *settlementTestQueue) CompleteSettlement(context.Context, domain.SettlementEntry) error {
	if q.completeError != nil {
		err := q.completeError
		q.completeError = nil
		return err
	}
	q.ready = true
	return nil
}
func (q *settlementTestQueue) FailSettlement(_ context.Context, _ domain.SettlementEntry, _ string, _ time.Duration, review bool) error {
	q.review = review
	q.failures++
	q.entry.Attempts++
	return nil
}
func (q *settlementTestQueue) VerifySettledEvents(context.Context, []domain.Event) error {
	if !q.ready {
		return domain.ErrSettlementPending
	}
	return nil
}
func (q *settlementTestQueue) ReleaseSettlements(context.Context, []domain.SettlementEntry) error {
	return nil
}

type interruptedSettler struct {
	actual        decisiondomain.ImpressionSettler
	before, after bool
}

func (s *interruptedSettler) SettleImpression(ctx context.Context, entry decisiondomain.Settlement) error {
	if s.before {
		s.before = false
		return errors.New("Redis temporarily unavailable")
	}
	if err := s.actual.SettleImpression(ctx, entry); err != nil {
		return err
	}
	if s.after {
		s.after = false
		return errors.New("Redis acknowledgement lost")
	}
	return nil
}

func TestSettlementWorkerRecoversWithoutClientRetry(t *testing.T) {
	for _, failure := range []string{"redis-unavailable", "redis-ack-lost", "mysql-completion-failed"} {
		t.Run(failure, func(t *testing.T) {
			server := miniredis.RunT(t)
			now := time.Now().UTC()
			server.SetTime(now)
			client := redis.NewClient(&redis.Options{Addr: server.Addr()})
			defer client.Close()
			r := decisionredis.NewReservations(client, "worker-test")
			if _, ok, err := r.ReserveFrequency(t.Context(), "user", "campaign", "token", 1, now, 30*time.Second); err != nil || !ok {
				t.Fatal(err)
			}
			if _, ok, err := r.ReserveBudget(t.Context(), "campaign", 10, 5, "token", now, 30*time.Second); err != nil || !ok {
				t.Fatal(err)
			}
			d := decisiondomain.Result{RequestID: "request", UserID: "user", CampaignID: "campaign", CreativeID: "creative", Matched: true, ReservationToken: "token", ExpiresAt: now.Add(30 * time.Second), Pricing: decisiondomain.Pricing{PriceFen: 5}}
			q := &settlementTestQueue{entry: domain.SettlementEntry{Settlement: decisiondomain.Settlement{EventID: "event", Decision: d}}}
			s := &interruptedSettler{actual: r, before: failure == "redis-unavailable", after: failure == "redis-ack-lost"}
			if failure == "mysql-completion-failed" {
				q.completeError = errors.New("MySQL connection lost")
			}
			worker := NewSettlementWorker(q, s, nil)
			store := eventmemory.NewStore()
			record := RecordSettledBatch(q, NewService(store, fixedDecisionFinder{d}, nil))
			events := []domain.Event{{EventID: "event", RequestID: "request", CampaignID: "campaign", CreativeID: "creative", Type: domain.Impression, OccurredAt: now}}
			if count, _ := worker.RunOnce(t.Context()); count != 0 || q.ready {
				t.Fatal("failed settlement made publishable")
			}
			if err := record(t.Context(), events); !errors.Is(err, domain.ErrSettlementPending) {
				t.Fatalf("unsettled consumer: %v", err)
			}
			if metrics, _ := store.Metrics(t.Context(), "campaign"); metrics.Impressions != 0 {
				t.Fatal("counted before settlement")
			}
			// A fresh server worker resumes the durable item; there is no HTTP retry.
			restarted := NewSettlementWorker(q, s, nil)
			if count, err := restarted.RunOnce(t.Context()); count != 1 || err != nil {
				t.Fatalf("count=%d err=%v", count, err)
			}
			for range 2 {
				if err := record(t.Context(), events); err != nil {
					t.Fatal(err)
				}
			}
			if metrics, _ := store.Metrics(t.Context(), "campaign"); metrics.Impressions != 1 {
				t.Fatalf("impressions=%d", metrics.Impressions)
			}
			if spent, _ := r.DebugBudgetSpent(t.Context(), "campaign", now); spent != 5 {
				t.Fatalf("spent=%d", spent)
			}
		})
	}
}

type missingSettlement struct{}

func (missingSettlement) SettleImpression(context.Context, decisiondomain.Settlement) error {
	return decisiondomain.ErrSettlementUnavailable
}
func TestSettlementWorkerQuarantinesLostReservation(t *testing.T) {
	q := &settlementTestQueue{}
	if count, err := NewSettlementWorker(q, missingSettlement{}, nil).RunOnce(t.Context()); count != 0 || err != nil || !q.review || q.ready {
		t.Fatalf("queue=%+v count=%d err=%v", q, count, err)
	}
}
