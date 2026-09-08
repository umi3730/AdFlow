package application

import (
	"context"
	"errors"
	"testing"
	"time"

	decisionmemory "github.com/zhanghaiyang/adflow/internal/decision/adapter/memory"
	decisiondomain "github.com/zhanghaiyang/adflow/internal/decision/domain"
	eventmemory "github.com/zhanghaiyang/adflow/internal/event/adapter/memory"
	"github.com/zhanghaiyang/adflow/internal/event/domain"
)

func testService(t *testing.T) (*Service, *eventmemory.Store) {
	t.Helper()
	ctx := context.Background()
	decisions := decisionmemory.NewRuntime()
	result := decisiondomain.Result{
		RequestID: "request-1", UserID: "user-1", Matched: true, CampaignID: "campaign-1",
		CreativeID: "creative-1", ReservationToken: "request-1", ExpiresAt: time.Now().Add(time.Minute),
		Pricing: decisiondomain.Pricing{Mode: "fixed", PriceFen: 5},
	}
	if _, ok, err := decisions.ReserveFrequency(ctx, "user-1", "campaign-1", "request-1", 100, time.Now(), time.Minute); err != nil || !ok {
		t.Fatal(err)
	}
	if _, ok, err := decisions.ReserveBudget(ctx, "campaign-1", 100, 5, "request-1", time.Now(), time.Minute); err != nil || !ok {
		t.Fatal(err)
	}
	if err := decisions.SaveDecision(ctx, result); err != nil {
		t.Fatal(err)
	}
	store := eventmemory.NewStore()
	return NewService(store, decisions, decisions), store
}

func TestRecordEventLifecycleAndMetrics(t *testing.T) {
	service, store := testService(t)
	ctx := context.Background()
	impression := domain.Event{EventID: "event-1", RequestID: "request-1", CampaignID: "campaign-1", CreativeID: "creative-1", Type: domain.Impression}
	created, err := service.Record(ctx, impression)
	if err != nil || !created {
		t.Fatalf("impression created=%v err=%v", created, err)
	}
	created, err = service.Record(ctx, impression)
	if err != nil || created {
		t.Fatalf("duplicate created=%v err=%v", created, err)
	}
	_, err = service.Record(ctx, domain.Event{EventID: "event-2", RequestID: "request-1", CampaignID: "campaign-1", CreativeID: "creative-1", Type: domain.Click})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Record(ctx, domain.Event{EventID: "event-3", RequestID: "request-1", CampaignID: "campaign-1", CreativeID: "creative-1", Type: domain.Conversion, ValueFen: 500})
	if err != nil {
		t.Fatal(err)
	}
	metrics, err := store.Metrics(ctx, "campaign-1")
	if err != nil {
		t.Fatal(err)
	}
	if metrics.Impressions != 1 || metrics.Clicks != 1 || metrics.Conversions != 1 || metrics.ValueFen != 500 {
		t.Fatalf("unexpected metrics: %+v", metrics)
	}
}

func TestClickRequiresImpression(t *testing.T) {
	service, _ := testService(t)
	_, err := service.Record(context.Background(), domain.Event{
		EventID: "event-1", RequestID: "request-1", CampaignID: "campaign-1", CreativeID: "creative-1", Type: domain.Click,
	})
	if !errors.Is(err, domain.ErrImpressionRequired) {
		t.Fatalf("error = %v", err)
	}
}

func TestRecordBatchPreservesImpressionBeforeClick(t *testing.T) {
	service, store := testService(t)
	err := service.RecordBatch(context.Background(), []domain.Event{
		{EventID: "batch-impression", RequestID: "request-1", CampaignID: "campaign-1", CreativeID: "creative-1", Type: domain.Impression},
		{EventID: "batch-click", RequestID: "request-1", CampaignID: "campaign-1", CreativeID: "creative-1", Type: domain.Click},
	})
	if err != nil {
		t.Fatal(err)
	}
	metrics, err := store.Metrics(context.Background(), "campaign-1")
	if err != nil || metrics.Impressions != 1 || metrics.Clicks != 1 {
		t.Fatalf("metrics=%+v err=%v", metrics, err)
	}
}
