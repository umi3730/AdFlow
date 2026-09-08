package redisadapter

import (
	"errors"
	"github.com/umi3730/adflow/internal/decision/domain"
	"sync"
	"testing"
	"time"
)

func reserveSettlement(t *testing.T, r *Reservations, now time.Time) domain.Settlement {
	t.Helper()
	if _, ok, err := r.ReserveFrequency(t.Context(), "user", "campaign", "token", 1, now, 30*time.Second); err != nil || !ok {
		t.Fatalf("frequency: %v %v", ok, err)
	}
	if _, ok, err := r.ReserveBudget(t.Context(), "campaign", 10, 5, "token", now, 30*time.Second); err != nil || !ok {
		t.Fatalf("budget: %v %v", ok, err)
	}
	return domain.Settlement{EventID: "impression", Decision: domain.Result{RequestID: "request", ReservationToken: "token", UserID: "user", CampaignID: "campaign", Pricing: domain.Pricing{PriceFen: 5}}}
}

func TestAtomicSettlementConcurrentRetriesChargeOnce(t *testing.T) {
	r, server := testReservations(t)
	now := time.Now().UTC()
	server.SetTime(now)
	settlement := reserveSettlement(t, r, now)
	errs := make(chan error, 32)
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() { errs <- r.SettleImpression(t.Context(), settlement) })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if spent, err := r.DebugBudgetSpent(t.Context(), "campaign", now); err != nil || spent != 5 {
		t.Fatalf("spent=%d err=%v", spent, err)
	}
	if _, ok, err := r.ReserveFrequency(t.Context(), "user", "campaign", "other", 1, now, 30*time.Second); err != nil || ok {
		t.Fatalf("frequency lost: %v %v", ok, err)
	}
	server.FastForward(time.Minute)
	server.SetTime(now.Add(time.Minute))
	if err := r.SettleImpression(t.Context(), settlement); err != nil {
		t.Fatalf("retry after original hold expiry: %v", err)
	}
	settlement.EventID = "another-impression"
	if err := r.SettleImpression(t.Context(), settlement); !errors.Is(err, domain.ErrSettlementUnavailable) {
		t.Fatalf("receipt rebound: %v", err)
	}
}

func TestAtomicSettlementMissingFrequencyDoesNotChargeBudget(t *testing.T) {
	r, server := testReservations(t)
	now := time.Now().UTC()
	server.SetTime(now)
	settlement := reserveSettlement(t, r, now)
	if err := r.ReleaseFrequency(t.Context(), "token"); err != nil {
		t.Fatal(err)
	}
	if err := r.SettleImpression(t.Context(), settlement); !errors.Is(err, domain.ErrSettlementUnavailable) {
		t.Fatal(err)
	}
	if spent, _ := r.DebugBudgetSpent(t.Context(), "campaign", now); spent != 0 {
		t.Fatalf("partial charge %d", spent)
	}
	if !server.Exists("adflow:test:budget:rsv:token") {
		t.Fatal("budget hold was mutated")
	}
}

func TestAtomicSettlementExpiredHoldRequiresReconciliation(t *testing.T) {
	r, server := testReservations(t)
	now := time.Now().UTC()
	server.SetTime(now)
	settlement := reserveSettlement(t, r, now)
	server.FastForward(31 * time.Second)
	server.SetTime(now.Add(31 * time.Second))
	if err := r.SettleImpression(t.Context(), settlement); !errors.Is(err, domain.ErrSettlementUnavailable) {
		t.Fatal(err)
	}
	if spent, _ := r.DebugBudgetSpent(t.Context(), "campaign", now); spent != 0 {
		t.Fatalf("expired hold charged %d", spent)
	}
}

func TestAtomicSettlementRejectsChangedWinningPriceAndUser(t *testing.T) {
	for _, field := range []string{"price", "user"} {
		t.Run(field, func(t *testing.T) {
			r, server := testReservations(t)
			now := time.Now().UTC()
			server.SetTime(now)
			settlement := reserveSettlement(t, r, now)
			if field == "price" {
				settlement.Decision.Pricing.PriceFen++
			} else {
				settlement.Decision.UserID = "other"
			}
			if err := r.SettleImpression(t.Context(), settlement); !errors.Is(err, domain.ErrSettlementUnavailable) {
				t.Fatal(err)
			}
			if spent, _ := r.DebugBudgetSpent(t.Context(), "campaign", now); spent != 0 {
				t.Fatalf("bad snapshot charged %d", spent)
			}
		})
	}
}
