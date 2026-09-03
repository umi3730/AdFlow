package redisadapter

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func testReservations(t *testing.T) (*Reservations, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewReservations(client, "adflow:test"), server
}

func TestRedisFrequencyReserveConfirmAndCap(t *testing.T) {
	reservations, _ := testReservations(t)
	now := time.Date(2026, 9, 2, 1, 0, 0, 0, time.UTC)
	if _, allowed, err := reservations.ReserveFrequency(context.Background(), "user", "campaign", "request-1", 1, now, time.Minute); err != nil || !allowed {
		t.Fatalf("first reserve: allowed=%v err=%v", allowed, err)
	}
	if err := reservations.ConfirmFrequency(context.Background(), "request-1", now); err != nil {
		t.Fatal(err)
	}
	if _, allowed, err := reservations.ReserveFrequency(context.Background(), "user", "campaign", "request-2", 1, now, time.Minute); err != nil || allowed {
		t.Fatalf("second reserve: allowed=%v err=%v", allowed, err)
	}
}

func TestRedisBudgetReserveReleaseAndConfirm(t *testing.T) {
	reservations, _ := testReservations(t)
	now := time.Date(2026, 9, 2, 1, 0, 0, 0, time.UTC)
	if _, allowed, err := reservations.ReserveBudget(context.Background(), "campaign", 100, 60, "request-1", now, time.Minute); err != nil || !allowed {
		t.Fatalf("first reserve: allowed=%v err=%v", allowed, err)
	}
	if _, allowed, _ := reservations.ReserveBudget(context.Background(), "campaign", 100, 60, "request-2", now, time.Minute); allowed {
		t.Fatal("second reserve should exceed budget")
	}
	if err := reservations.ReleaseBudget(context.Background(), "request-1"); err != nil {
		t.Fatal(err)
	}
	if _, allowed, err := reservations.ReserveBudget(context.Background(), "campaign", 100, 60, "request-2", now, time.Minute); err != nil || !allowed {
		t.Fatalf("reserve after release: allowed=%v err=%v", allowed, err)
	}
	if err := reservations.ConfirmBudget(context.Background(), "request-2", now); err != nil {
		t.Fatal(err)
	}
	spent, err := reservations.DebugBudgetSpent(context.Background(), "campaign", now)
	if err != nil || spent != 60 {
		t.Fatalf("spent=%d err=%v", spent, err)
	}
}

func TestRedisReservationsUseRelativeExpiryAcrossHostClockSkew(t *testing.T) {
	reservations, server := testReservations(t)
	appNow := time.Date(2026, 9, 2, 1, 0, 0, 0, time.UTC)
	server.SetTime(appNow.Add(72 * time.Hour))

	if _, allowed, err := reservations.ReserveBudget(context.Background(), "campaign", 100, 100, "budget-request", appNow, time.Minute); err != nil || !allowed {
		t.Fatalf("reserve budget: allowed=%v err=%v", allowed, err)
	}
	if err := reservations.ConfirmBudget(context.Background(), "budget-request", appNow); err != nil {
		t.Fatal(err)
	}
	spent, err := reservations.DebugBudgetSpent(context.Background(), "campaign", appNow)
	if err != nil || spent != 100 {
		t.Fatalf("spent=%d err=%v", spent, err)
	}

	if _, allowed, err := reservations.ReserveFrequency(context.Background(), "user", "campaign", "frequency-request", 1, appNow, time.Minute); err != nil || !allowed {
		t.Fatalf("reserve frequency: allowed=%v err=%v", allowed, err)
	}
	if err := reservations.ConfirmFrequency(context.Background(), "frequency-request", appNow); err != nil {
		t.Fatal(err)
	}
	if _, allowed, err := reservations.ReserveFrequency(context.Background(), "user", "campaign", "frequency-request-2", 1, appNow, time.Minute); err != nil || allowed {
		t.Fatalf("frequency cap after confirm: allowed=%v err=%v", allowed, err)
	}
}
