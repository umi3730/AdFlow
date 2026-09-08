package memory

import (
	"github.com/umi3730/adflow/internal/decision/domain"
	"testing"
	"time"
)

func TestPeekDecisionDoesNotCleanExpiredReservations(t *testing.T) {
	r := NewRuntime()
	now := time.Now().UTC()
	if _, ok, err := r.ReserveBudget(t.Context(), "campaign", 100, 5, "old", now.Add(-time.Minute), time.Second); err != nil || !ok {
		t.Fatal(err)
	}
	if err := r.SaveDecision(t.Context(), domain.Result{RequestID: "r"}); err != nil {
		t.Fatal(err)
	}
	if _, found, err := r.PeekDecision(t.Context(), "r"); err != nil || !found {
		t.Fatal(err)
	}
	if _, exists := r.budgetReservations["old"]; !exists {
		t.Fatal("inspection released expired reservations")
	}
}
