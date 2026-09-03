package memory

import (
	"context"
	"testing"
	"time"
)

func TestFrequencyReservationIsAtomic(t *testing.T) {
	runtime := NewRuntime()
	now := time.Now().UTC()
	if _, allowed, _ := runtime.ReserveFrequency(context.Background(), "user", "campaign", "request-1", 1, now, time.Minute); !allowed {
		t.Fatal("first reservation should pass")
	}
	if _, allowed, _ := runtime.ReserveFrequency(context.Background(), "user", "campaign", "request-2", 1, now, time.Minute); allowed {
		t.Fatal("second reservation should be capped")
	}
}

func TestBudgetReservationPreventsOverspend(t *testing.T) {
	runtime := NewRuntime()
	now := time.Now().UTC()
	if _, allowed, _ := runtime.ReserveBudget(context.Background(), "campaign", 100, 60, "request-1", now, time.Minute); !allowed {
		t.Fatal("first reservation should pass")
	}
	if _, allowed, _ := runtime.ReserveBudget(context.Background(), "campaign", 100, 60, "request-2", now, time.Minute); allowed {
		t.Fatal("second reservation should exceed budget")
	}
}
