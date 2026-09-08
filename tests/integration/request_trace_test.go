//go:build integration

package integration

import (
	"context"
	"errors"
	decisionmysql "github.com/umi3730/adflow/internal/decision/adapter/mysql"
	decisiondomain "github.com/umi3730/adflow/internal/decision/domain"
	eventmysql "github.com/umi3730/adflow/internal/event/adapter/mysql"
	"github.com/umi3730/adflow/internal/event/domain"
	operations "github.com/umi3730/adflow/internal/operations/application"
	"strings"
	"testing"
	"time"
)

func TestRequestTraceReadsDurableStagesWithoutAdvancingThem(t *testing.T) {
	db := integrationMySQL(t)
	id := "trace-" + newID(t)
	campaignID := newID(t)
	eventID := newID(t)
	t.Cleanup(func() {
		for _, query := range []string{"DELETE FROM processed_events WHERE request_id = ?", "DELETE FROM event_outbox WHERE aggregate_key = ?", "DELETE FROM event_receipts WHERE request_id = ?", "DELETE FROM event_settlements WHERE request_id = ?", "DELETE FROM decisions WHERE request_id = ?"} {
			if _, err := db.ExecContext(context.Background(), query, id); err != nil {
				t.Error(err)
			}
		}
		if _, err := db.ExecContext(context.Background(), "DELETE FROM campaign_metrics WHERE campaign_id = ?", campaignID); err != nil {
			t.Error(err)
		}
	})
	decisions := decisionmysql.NewStore(db)
	outbox := eventmysql.NewOutbox(db, "trace-owner")
	metrics := eventmysql.NewStore(db)
	now := time.Now().UTC()
	decision := decisiondomain.Result{RequestID: id, UserID: "trace-user", SlotID: "trace-slot", CampaignID: campaignID, CreativeID: campaignID, Matched: true, ReservationToken: "private-token", ExpiresAt: now.Add(time.Minute), Pricing: decisiondomain.Pricing{Mode: "first_price", PriceFen: 5, Version: 2}}
	if err := decisions.SaveDecision(t.Context(), decision); err != nil {
		t.Fatal(err)
	}
	service := operations.NewTraceService(decisions, outbox, "kafka")
	before, err := service.Read(t.Context(), id)
	if err != nil || before.Decision == nil || before.Settlement != nil || len(before.Events) != 0 {
		t.Fatalf("before=%+v err=%v", before, err)
	}
	event := domain.Event{EventID: eventID, RequestID: id, CampaignID: campaignID, CreativeID: campaignID, Type: domain.Impression, OccurredAt: now}
	if _, err := outbox.EnqueueForSettlement(t.Context(), event, decision, now); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		pending, err := service.Read(t.Context(), id)
		if err != nil || pending.Settlement.Status != "PENDING" || pending.Events[0].Status != "SETTLING" || pending.Events[0].ProcessedAt != nil {
			t.Fatalf("pending=%+v err=%v", pending, err)
		}
	}
	// Construct persisted stages explicitly; this is a read-projection test,
	// not a claim that calling inspection performed Redis settlement or publishing.
	claims, err := outbox.ClaimSettlements(t.Context(), 1, time.Minute)
	if err != nil || len(claims) != 1 {
		t.Fatalf("claims=%v err=%v", claims, err)
	}
	if err := outbox.CompleteSettlement(t.Context(), claims[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := outbox.ClaimBatch(t.Context(), 1, now, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := outbox.MarkPublished(t.Context(), eventID, now); err != nil {
		t.Fatal(err)
	}
	published, err := service.Read(t.Context(), id)
	if err != nil || published.Events[0].PublishedAt == nil || published.Events[0].ProcessedAt != nil {
		t.Fatalf("publication confused with metrics: %+v %v", published, err)
	}
	if _, err := metrics.Record(t.Context(), event); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), "DELETE FROM decisions WHERE request_id = ?", id); err != nil {
		t.Fatal(err)
	}
	completed, err := service.Read(t.Context(), id)
	if err != nil || completed.Decision == nil || completed.Decision.Pricing.PriceFen != 5 || completed.Events[0].ProcessedAt == nil {
		t.Fatalf("receipt fallback=%+v err=%v", completed, err)
	}
	if _, err := service.Read(t.Context(), strings.ToUpper(id)); !errors.Is(err, operations.ErrTraceNotFound) {
		t.Fatalf("case alias resolved: %v", err)
	}
	for range 101 {
		click := event
		click.EventID = newID(t)
		click.Type = domain.Click
		if _, err := outbox.EnqueueForSettlement(t.Context(), click, decision, now); err != nil {
			t.Fatal(err)
		}
	}
	capped, err := service.Read(t.Context(), id)
	if err != nil || len(capped.Events) != domain.TraceEventLimit || !capped.Truncated {
		t.Fatalf("cap=%d truncated=%v err=%v", len(capped.Events), capped.Truncated, err)
	}
	totals, err := metrics.Metrics(t.Context(), campaignID)
	if err != nil || totals.Impressions != 1 || totals.Clicks != 0 {
		t.Fatalf("inspection changed metrics: %+v %v", totals, err)
	}
}
