package application

import (
	"context"
	"errors"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	decisionmemory "github.com/zhanghaiyang/adflow/internal/decision/adapter/memory"
	decisionredis "github.com/zhanghaiyang/adflow/internal/decision/adapter/redis"
	decisiondomain "github.com/zhanghaiyang/adflow/internal/decision/domain"
	eventmemory "github.com/zhanghaiyang/adflow/internal/event/adapter/memory"
	"github.com/zhanghaiyang/adflow/internal/event/domain"
	"sync"
	"testing"
	"time"
)

func receiptFixture(t *testing.T) (*Service, *decisionredis.Reservations, *miniredis.Miniredis, time.Time) {
	t.Helper()
	now := time.Now().UTC()
	server := miniredis.RunT(t)
	server.SetTime(now)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	reservations := decisionredis.NewReservations(client, "receipt-test")
	decisions := decisionmemory.NewRuntime()
	for _, id := range []string{"r1", "r2"} {
		if _, ok, err := reservations.ReserveFrequency(t.Context(), "u", "campaign", id, 100, now, 30*time.Second); err != nil || !ok {
			t.Fatal(err)
		}
		if _, ok, err := reservations.ReserveBudget(t.Context(), "campaign", 100, 5, id, now, 30*time.Second); err != nil || !ok {
			t.Fatal(err)
		}
		if err := decisions.SaveDecision(t.Context(), decisiondomain.Result{RequestID: id, UserID: "u", Matched: true, CampaignID: "campaign", CreativeID: "creative", ReservationToken: id, ExpiresAt: now.Add(30 * time.Second), Pricing: decisiondomain.Pricing{Mode: "fixed", PriceFen: 5}}); err != nil {
			t.Fatal(err)
		}
	}
	service := NewService(eventmemory.NewStore(), decisions, reservations)
	service.now = func() time.Time { return now }
	return service, reservations, server, now
}
func receiptEvent(id, request string, kind domain.Type, now time.Time) domain.Event {
	return domain.Event{EventID: id, RequestID: request, CampaignID: "campaign", CreativeID: "creative", Type: kind, OccurredAt: now}
}

func TestSyncEventCollisionCannotSettleAnotherDecision(t *testing.T) {
	service, r, _, now := receiptFixture(t)
	if _, err := service.Record(t.Context(), receiptEvent("same", "r1", domain.Impression, now)); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Record(t.Context(), receiptEvent("same", "r2", domain.Impression, now)); !errors.Is(err, domain.ErrEventConflict) {
		t.Fatalf("collision error=%v", err)
	}
	m, _ := service.Metrics(t.Context(), "campaign")
	spent, _ := r.DebugBudgetSpent(t.Context(), "campaign", now)
	if m.Impressions != 1 || spent != 5 {
		t.Fatalf("impressions=%d spent=%d", m.Impressions, spent)
	}
}
func TestSyncSingleDecisionHasOnlyOneImpression(t *testing.T) {
	service, r, _, now := receiptFixture(t)
	if _, err := service.Record(t.Context(), receiptEvent("first", "r1", domain.Impression, now)); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Record(t.Context(), receiptEvent("second", "r1", domain.Impression, now)); !errors.Is(err, domain.ErrEventConflict) {
		t.Fatal(err)
	}
	m, _ := service.Metrics(t.Context(), "campaign")
	spent, _ := r.DebugBudgetSpent(t.Context(), "campaign", now)
	if m.Impressions != 1 || spent != 5 {
		t.Fatalf("impressions=%d spent=%d", m.Impressions, spent)
	}
}
func TestSyncConcurrentDuplicatesSettleAndCountOnce(t *testing.T) {
	service, r, _, now := receiptFixture(t)
	event := receiptEvent("same", "r1", domain.Impression, now)
	var wg sync.WaitGroup
	results := make(chan bool, 32)
	errs := make(chan error, 32)
	for range 32 {
		wg.Go(func() { created, err := service.Record(t.Context(), event); results <- created; errs <- err })
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	count := 0
	for created := range results {
		if created {
			count++
		}
	}
	m, _ := service.Metrics(t.Context(), "campaign")
	spent, _ := r.DebugBudgetSpent(t.Context(), "campaign", now)
	if count != 1 || m.Impressions != 1 || spent != 5 {
		t.Fatalf("created=%d impressions=%d spent=%d", count, m.Impressions, spent)
	}
}

type loseReceiptAck struct {
	actual decisiondomain.ImpressionSettler
	lost   bool
}

func (s *loseReceiptAck) SettleImpression(ctx context.Context, entry decisiondomain.Settlement) error {
	if err := s.actual.SettleImpression(ctx, entry); err != nil {
		return err
	}
	if !s.lost {
		s.lost = true
		return errors.New("acknowledgement lost")
	}
	return nil
}
func TestSyncSettlementAckLossRetriesOriginalReceiptAfterExpiry(t *testing.T) {
	service, r, server, now := receiptFixture(t)
	service.confirmer = &loseReceiptAck{actual: r}
	event := receiptEvent("same", "r1", domain.Impression, now)
	if _, err := service.Record(t.Context(), event); err == nil {
		t.Fatal("expected lost acknowledgement")
	}
	m, _ := service.Metrics(t.Context(), "campaign")
	if m.Impressions != 0 {
		t.Fatal("unconfirmed response counted")
	}
	server.FastForward(time.Minute)
	server.SetTime(now.Add(time.Minute))
	service.now = func() time.Time { return now.Add(time.Minute) }
	if created, err := service.Record(t.Context(), event); err != nil || !created {
		t.Fatalf("retry created=%v err=%v", created, err)
	}
	spent, _ := r.DebugBudgetSpent(t.Context(), "campaign", now)
	m, _ = service.Metrics(t.Context(), "campaign")
	if spent != 5 || m.Impressions != 1 {
		t.Fatalf("spent=%d impressions=%d", spent, m.Impressions)
	}
}
func TestSyncExpiredUnconfirmedReservationDoesNotCount(t *testing.T) {
	service, r, server, now := receiptFixture(t)
	server.FastForward(time.Minute)
	if _, err := service.Record(t.Context(), receiptEvent("missing", "r1", domain.Impression, now)); !errors.Is(err, decisiondomain.ErrSettlementUnavailable) {
		t.Fatal(err)
	}
	m, _ := service.Metrics(t.Context(), "campaign")
	spent, _ := r.DebugBudgetSpent(t.Context(), "campaign", now)
	if m.Impressions != 0 || spent != 0 {
		t.Fatalf("impressions=%d spent=%d", m.Impressions, spent)
	}
}

type unavailableDecisionFinder struct{}

func (unavailableDecisionFinder) FindDecision(context.Context, string) (decisiondomain.Result, bool, error) {
	return decisiondomain.Result{}, false, errors.New("decision no longer retained")
}
func TestSyncAttributionUsesImpressionAfterDecisionExpires(t *testing.T) {
	service, r, _, now := receiptFixture(t)
	if _, err := service.Record(t.Context(), receiptEvent("impression", "r1", domain.Impression, now)); err != nil {
		t.Fatal(err)
	}
	service.decisions = unavailableDecisionFinder{}
	service.now = func() time.Time { return now.Add(time.Hour) }
	for _, kind := range []domain.Type{domain.Click, domain.Conversion} {
		if _, err := service.Record(t.Context(), receiptEvent(string(kind), "r1", kind, now.Add(time.Hour))); err != nil {
			t.Fatal(err)
		}
	}
	service.now = func() time.Time { return now.Add(domain.DefaultAttributionWindow) }
	if _, err := service.Record(t.Context(), receiptEvent("too-late", "r1", domain.Click, now.Add(domain.DefaultAttributionWindow))); !errors.Is(err, domain.ErrAttributionExpired) {
		t.Fatal(err)
	}
	if created, err := service.Record(t.Context(), receiptEvent("click", "r1", domain.Click, now.Add(time.Hour))); err != nil || created {
		t.Fatalf("late retry created=%v err=%v", created, err)
	}
	spent, _ := r.DebugBudgetSpent(t.Context(), "campaign", now)
	if spent != 5 {
		t.Fatalf("callbacks changed spent=%d", spent)
	}
}
func TestAsyncLaterCallbacksAndConsumerReplay(t *testing.T) {
	now := time.Now().UTC()
	d := decisiondomain.Result{RequestID: "r", Matched: true, CampaignID: "campaign", CreativeID: "creative", ExpiresAt: now.Add(30 * time.Second)}
	store := eventmemory.NewStore()
	processor := NewService(store, fixedDecisionFinder{d}, nil)
	outbox := &recordingOutbox{}
	ingress := NewAsyncService(processor, fixedDecisionFinder{d}, outbox)
	ingress.now = func() time.Time { return now }
	impression := receiptEvent("impression", "r", domain.Impression, now)
	if _, err := ingress.Record(t.Context(), impression); err != nil {
		t.Fatal(err)
	}
	if _, err := processor.Record(t.Context(), impression); err != nil {
		t.Fatal(err)
	}
	ingress.decisions = unavailableDecisionFinder{}
	ingress.now = func() time.Time { return now.Add(time.Hour) }
	click := receiptEvent("click", "r", domain.Click, now.Add(time.Hour))
	if _, err := ingress.Record(t.Context(), click); err != nil {
		t.Fatal(err)
	}
	processor.now = func() time.Time { return now.Add(8 * 24 * time.Hour) }
	if err := processor.RecordBatch(t.Context(), []domain.Event{click}); err != nil {
		t.Fatal(err)
	}
	ingress.now = func() time.Time { return now.Add(domain.DefaultAttributionWindow) }
	if _, err := ingress.Record(t.Context(), receiptEvent("late", "r", domain.Conversion, now.Add(domain.DefaultAttributionWindow))); !errors.Is(err, domain.ErrAttributionExpired) {
		t.Fatal(err)
	}
	if _, err := ingress.Record(t.Context(), receiptEvent("no-impression", "other", domain.Click, now)); !errors.Is(err, domain.ErrImpressionRequired) {
		t.Fatal(err)
	}
}
