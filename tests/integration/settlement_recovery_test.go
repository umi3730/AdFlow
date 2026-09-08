//go:build integration

package integration

import (
	"context"
	"errors"
	"github.com/redis/go-redis/v9"
	decisionredis "github.com/zhanghaiyang/adflow/internal/decision/adapter/redis"
	decisiondomain "github.com/zhanghaiyang/adflow/internal/decision/domain"
	eventmysql "github.com/zhanghaiyang/adflow/internal/event/adapter/mysql"
	eventdomain "github.com/zhanghaiyang/adflow/internal/event/domain"
	"testing"
	"time"
)

// Run against a dedicated integration database, never the interactive demo DB.
func TestSettlementRestartFencesOldWorkerAndReleasesDependentEvents(t *testing.T) {
	db := integrationMySQL(t)
	client := redis.NewClient(&redis.Options{Addr: integrationEnv(t, "ADFLOW_IT_REDIS_ADDR")})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	prefix := "adflow:it:settlement:" + newID(t)
	requestID, eventID := newID(t), newID(t)
	clickID, conversionID := newID(t), newID(t)
	now := time.Now().UTC()
	d := decisiondomain.Result{RequestID: requestID, UserID: newID(t), CampaignID: newID(t), CreativeID: newID(t), Matched: true, ReservationToken: newID(t), ExpiresAt: now.Add(time.Minute), Pricing: decisiondomain.Pricing{Mode: "first_price", PriceFen: 5}}
	impression := eventdomain.Event{EventID: eventID, RequestID: requestID, CampaignID: d.CampaignID, CreativeID: d.CreativeID, Type: eventdomain.Impression, OccurredAt: now}
	click, conversion := impression, impression
	click.EventID, click.Type = clickID, eventdomain.Click
	conversion.EventID, conversion.Type = conversionID, eventdomain.Conversion
	t.Cleanup(func() {
		if err := deletePrefix(context.Background(), client, prefix); err != nil {
			t.Errorf("clean Redis test keys: %v", err)
		}
		_, _ = db.Exec(`DELETE FROM event_outbox WHERE aggregate_key = ?`, requestID)
		_, _ = db.Exec(`DELETE FROM event_receipts WHERE request_id = ?`, requestID)
		_, _ = db.Exec(`DELETE FROM event_settlements WHERE request_id = ?`, requestID)
	})
	r := decisionredis.NewReservations(client, prefix)
	if _, ok, err := r.ReserveFrequency(t.Context(), d.UserID, d.CampaignID, d.ReservationToken, 1, now, time.Minute); err != nil || !ok {
		t.Fatal(err)
	}
	if _, ok, err := r.ReserveBudget(t.Context(), d.CampaignID, 10, 5, d.ReservationToken, now, time.Minute); err != nil || !ok {
		t.Fatal(err)
	}
	first, second := eventmysql.NewOutbox(db, "settlement-first"), eventmysql.NewOutbox(db, "settlement-second")
	for _, event := range []eventdomain.Event{impression, click} {
		if created, err := first.EnqueueForSettlement(t.Context(), event, d, now); err != nil || !created {
			t.Fatalf("created=%v err=%v", created, err)
		}
	}
	if err := first.VerifySettledEvents(t.Context(), []eventdomain.Event{impression}); !errors.Is(err, eventdomain.ErrSettlementPending) {
		t.Fatal(err)
	}
	if entries, err := first.ClaimBatch(t.Context(), 10, now, time.Minute); err != nil || len(entries) != 0 {
		t.Fatalf("unsettled events published: %v %v", entries, err)
	}
	claims, err := first.ClaimSettlements(t.Context(), 1, time.Minute)
	if err != nil || len(claims) != 1 {
		t.Fatalf("claims=%v err=%v", claims, err)
	}
	old := claims[0]
	if claims, err := second.ClaimSettlements(t.Context(), 1, time.Minute); err != nil || len(claims) != 0 {
		t.Fatalf("stole active lease: %v %v", claims, err)
	}
	// Redis committed, but the process crashed before its MySQL completion.
	if err := r.SettleImpression(t.Context(), old.Settlement); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE event_settlements SET locked_until = UTC_TIMESTAMP(3) - INTERVAL 1 SECOND WHERE request_id = ?`, requestID); err != nil {
		t.Fatal(err)
	}
	claims, err = second.ClaimSettlements(t.Context(), 1, time.Minute)
	if err != nil || len(claims) != 1 {
		t.Fatalf("restart claims=%v err=%v", claims, err)
	}
	if err := first.CompleteSettlement(t.Context(), old); !errors.Is(err, eventdomain.ErrSettlementLeaseLost) {
		t.Fatalf("old worker completed: %v", err)
	}
	if err := r.SettleImpression(t.Context(), claims[0].Settlement); err != nil {
		t.Fatal(err)
	}
	// Race a dependent event against completion; either lock order must leave it publishable.
	results := make(chan error, 2)
	go func() { results <- second.CompleteSettlement(t.Context(), claims[0]) }()
	go func() { _, err := first.EnqueueForSettlement(t.Context(), conversion, d, now); results <- err }()
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	events := []eventdomain.Event{impression, click, conversion}
	if err := first.VerifySettledEvents(t.Context(), events); err != nil {
		t.Fatal(err)
	}
	entries, err := first.ClaimBatch(t.Context(), 10, time.Now(), time.Minute)
	if err != nil || len(entries) != 1 || entries[0].Event.EventID != eventID {
		t.Fatalf("published candidates=%v err=%v", entries, err)
	}
	if err := first.MarkPublished(t.Context(), eventID, time.Now()); err != nil {
		t.Fatal(err)
	}
	entries, err = second.ClaimBatch(t.Context(), 10, time.Now(), time.Minute)
	if err != nil || len(entries) != 2 {
		t.Fatalf("dependent events after impression publication=%v err=%v", entries, err)
	}
	if spent, err := r.DebugBudgetSpent(t.Context(), d.CampaignID, now); err != nil || spent != 5 {
		t.Fatalf("spent=%d err=%v", spent, err)
	}
}
