package memory

import (
	"errors"
	"github.com/umi3730/adflow/internal/decision/domain"
	"testing"
	"time"
)

func TestMemorySettlementKeepsBothCapsAndRejectsDifferentReceipt(t *testing.T) {
	r := NewRuntime()
	now := time.Now().UTC()
	if _, ok, err := r.ReserveFrequency(t.Context(), "user", "campaign", "token", 1, now, time.Minute); err != nil || !ok {
		t.Fatal(err)
	}
	if _, ok, err := r.ReserveBudget(t.Context(), "campaign", 5, 5, "token", now, time.Minute); err != nil || !ok {
		t.Fatal(err)
	}
	entry := domain.Settlement{EventID: "event", Decision: domain.Result{RequestID: "request", UserID: "user", CampaignID: "campaign", ReservationToken: "token", Pricing: domain.Pricing{PriceFen: 5}}}
	for range 2 {
		if err := r.SettleImpression(t.Context(), entry); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok, _ := r.ReserveFrequency(t.Context(), "user", "campaign", "next", 1, now, time.Minute); ok {
		t.Fatal("lost frequency count")
	}
	if _, ok, _ := r.ReserveBudget(t.Context(), "campaign", 5, 5, "next", now, time.Minute); ok {
		t.Fatal("lost budget charge")
	}
	entry.EventID = "other"
	if err := r.SettleImpression(t.Context(), entry); !errors.Is(err, domain.ErrSettlementUnavailable) {
		t.Fatal(err)
	}
}
