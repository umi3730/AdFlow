package application

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	decisionredis "github.com/zhanghaiyang/adflow/internal/decision/adapter/redis"
	decisiondomain "github.com/zhanghaiyang/adflow/internal/decision/domain"
	"github.com/zhanghaiyang/adflow/internal/event/domain"
)

type leasedBatchQueue struct {
	entries                          []domain.SettlementEntry
	states                           []string
	now, until                       time.Time
	claimedLease                     time.Duration
	completeErr, failErr, releaseErr error
	completeFailIndex                int
	releases                         int
	cleanupUsable                    bool
}

func (q *leasedBatchQueue) budgetTime() time.Time {
	return q.entries[0].Settlement.Decision.ExpiresAt.Add(-30 * time.Second)
}

func (q *leasedBatchQueue) ClaimSettlements(_ context.Context, limit int, lease time.Duration) ([]domain.SettlementEntry, error) {
	q.claimedLease = lease
	out := []domain.SettlementEntry{}
	for i, e := range q.entries {
		if len(out) >= limit {
			break
		}
		if q.states[i] == "PENDING" || (q.states[i] == "PROCESSING" && !q.now.Before(q.until)) {
			q.states[i] = "PROCESSING"
			out = append(out, e)
		}
	}
	if len(out) > 0 {
		q.until = q.now.Add(lease)
	}
	return out, nil
}
func (q *leasedBatchQueue) index(entry domain.SettlementEntry) int {
	for i, e := range q.entries {
		if e.Settlement.EventID == entry.Settlement.EventID {
			return i
		}
	}
	panic("unknown entry")
}
func (q *leasedBatchQueue) CompleteSettlement(ctx context.Context, e domain.SettlementEntry) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if q.completeErr != nil && q.index(e) == q.completeFailIndex {
		err := q.completeErr
		q.completeErr = nil
		return err
	}
	q.states[q.index(e)] = "SETTLED"
	return nil
}

type cancelAfterConfirm struct {
	actual decisiondomain.ImpressionSettler
	cancel context.CancelFunc
}

func (s cancelAfterConfirm) SettleImpression(ctx context.Context, settlement decisiondomain.Settlement) error {
	if err := s.actual.SettleImpression(ctx, settlement); err != nil {
		return err
	}
	s.cancel()
	return nil
}
func TestSettlementCancellationReturnsClaimsWithoutQuarantine(t *testing.T) {
	q, r, _ := batchRecoveryFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	_, err := NewSettlementWorker(q, cancelAfterConfirm{actual: r, cancel: cancel}, nil).RunOnce(ctx)
	if !errors.Is(err, context.Canceled) || !q.cleanupUsable {
		t.Fatalf("cancellation cleanup: %v %+v", err, q)
	}
	for _, state := range q.states {
		if state != "PENDING" {
			t.Fatalf("canceled batch state: %v", q.states)
		}
	}
	if n, err := NewSettlementWorker(q, r, nil).RunOnce(t.Context()); err != nil || n != 3 {
		t.Fatalf("resumed %d %v", n, err)
	}
	if spent, _ := r.DebugBudgetSpent(t.Context(), "campaign", q.budgetTime()); spent != 15 {
		t.Fatalf("spent=%d", spent)
	}
}
func TestSettlementBatchKeepsCompletedPrefix(t *testing.T) {
	q, r, _ := batchRecoveryFixture(t)
	q.completeFailIndex = 1
	q.completeErr = errors.New("second completion failed")
	if n, err := NewSettlementWorker(q, r, nil).RunOnce(t.Context()); err == nil || n != 1 {
		t.Fatalf("completed=%d err=%v", n, err)
	}
	if q.states[0] != "SETTLED" || q.states[1] != "PENDING" || q.states[2] != "PENDING" {
		t.Fatalf("states=%v", q.states)
	}
	if n, err := NewSettlementWorker(q, r, nil).RunOnce(t.Context()); err != nil || n != 2 {
		t.Fatalf("resumed=%d err=%v", n, err)
	}
	if spent, _ := r.DebugBudgetSpent(t.Context(), "campaign", q.budgetTime()); spent != 15 {
		t.Fatalf("spent=%d", spent)
	}
}
func (q *leasedBatchQueue) FailSettlement(ctx context.Context, e domain.SettlementEntry, _ string, _ time.Duration, review bool) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if q.failErr != nil {
		err := q.failErr
		q.failErr = nil
		return err
	}
	state := "PENDING"
	if review {
		state = "RECONCILE"
	}
	q.states[q.index(e)] = state
	return nil
}
func (q *leasedBatchQueue) ReleaseSettlements(ctx context.Context, entries []domain.SettlementEntry) error {
	q.releases++
	_, hasDeadline := ctx.Deadline()
	q.cleanupUsable = ctx.Err() == nil && hasDeadline
	if q.releaseErr != nil {
		return q.releaseErr
	}
	for _, e := range entries {
		i := q.index(e)
		if q.states[i] == "PROCESSING" {
			q.states[i] = "PENDING"
		}
	}
	return nil
}
func batchRecoveryFixture(t *testing.T) (*leasedBatchQueue, *decisionredis.Reservations, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	now := time.Now().UTC()
	server.SetTime(now)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	r := decisionredis.NewReservations(client, "batch-recovery")
	q := &leasedBatchQueue{now: now}
	for i := 0; i < 3; i++ {
		user, token, event := fmt.Sprintf("user-%d", i), fmt.Sprintf("token-%d", i), fmt.Sprintf("event-%d", i)
		if _, ok, err := r.ReserveFrequency(t.Context(), user, "campaign", token, 1, now, 30*time.Second); err != nil || !ok {
			t.Fatal(err)
		}
		if _, ok, err := r.ReserveBudget(t.Context(), "campaign", 15, 5, token, now, 30*time.Second); err != nil || !ok {
			t.Fatal(err)
		}
		q.entries = append(q.entries, domain.SettlementEntry{Owner: "owner", Settlement: decisiondomain.Settlement{EventID: event, Decision: decisiondomain.Result{RequestID: event, UserID: user, CampaignID: "campaign", ReservationToken: token, ExpiresAt: now.Add(30 * time.Second), Pricing: decisiondomain.Pricing{PriceFen: 5}}}})
		q.states = append(q.states, "PENDING")
	}
	return q, r, server
}
func TestSettlementBatchFailureReturnsUnfinishedClaims(t *testing.T) {
	for _, failure := range []string{"complete", "fail"} {
		t.Run(failure, func(t *testing.T) {
			q, r, server := batchRecoveryFixture(t)
			var settler decisiondomain.ImpressionSettler = r
			if failure == "complete" {
				q.completeErr = errors.New("database completion failed")
			} else {
				q.failErr = errors.New("database failure update failed")
				settler = &interruptedSettler{actual: r, before: true}
			}
			worker := NewSettlementWorker(q, settler, nil)
			if _, err := worker.RunOnce(t.Context()); err == nil {
				t.Fatal("missing failed persistence error")
			}
			if q.releases != 1 || !q.cleanupUsable {
				t.Fatalf("unfinished leases not released with bounded cleanup context: %+v", q)
			}
			for _, state := range q.states {
				if state != "PENDING" {
					t.Fatalf("stranded batch: %v", q.states)
				}
			}
			q.now = q.now.Add(time.Second)
			server.FastForward(time.Second)
			if n, err := NewSettlementWorker(q, r, nil).RunOnce(t.Context()); err != nil || n != 3 {
				t.Fatalf("recovery count=%d err=%v states=%v", n, err, q.states)
			}
			if spent, err := r.DebugBudgetSpent(t.Context(), "campaign", q.budgetTime()); err != nil || spent != 15 {
				t.Fatalf("double charge or missing charge: %d %v", spent, err)
			}
		})
	}
}
func TestSettlementBatchCleanupFailureHasShortLeaseFallback(t *testing.T) {
	q, r, server := batchRecoveryFixture(t)
	q.completeErr = errors.New("completion failed")
	cleanupErr := errors.New("cleanup unavailable")
	q.releaseErr = cleanupErr
	_, err := NewSettlementWorker(q, r, nil).RunOnce(t.Context())
	if !errors.Is(err, cleanupErr) {
		t.Fatalf("cleanup error hidden: %v", err)
	}
	if q.claimedLease >= 30*time.Second {
		t.Fatalf("lease outlives reservation: %v", q.claimedLease)
	}
	q.releaseErr = nil
	q.now = q.now.Add(16 * time.Second)
	server.FastForward(16 * time.Second)
	if n, err := NewSettlementWorker(q, r, nil).RunOnce(t.Context()); err != nil || n != 3 {
		t.Fatalf("lease fallback missed still-valid reservations: %d %v", n, err)
	}
	if spent, _ := r.DebugBudgetSpent(t.Context(), "campaign", q.budgetTime()); spent != 15 {
		t.Fatalf("spent=%d", spent)
	}
}
