package application

import (
	"context"
	"encoding/json"
	"errors"
	decisionmemory "github.com/zhanghaiyang/adflow/internal/decision/adapter/memory"
	decisiondomain "github.com/zhanghaiyang/adflow/internal/decision/domain"
	eventmemory "github.com/zhanghaiyang/adflow/internal/event/adapter/memory"
	"github.com/zhanghaiyang/adflow/internal/event/domain"
	"strings"
	"testing"
	"time"
)

type emptyInspector struct{}

func (emptyInspector) PeekDecision(context.Context, string) (decisiondomain.Result, bool, error) {
	return decisiondomain.Result{}, false, nil
}

type traceState struct {
	state domain.RequestState
	err   error
}

func (s traceState) ReadRequestState(context.Context, string) (domain.RequestState, error) {
	return s.state, s.err
}

func TestTraceNoAdAndUnknownRequestAreDistinct(t *testing.T) {
	decisions := decisionmemory.NewRuntime()
	events := eventmemory.NewStore()
	if err := decisions.SaveDecision(t.Context(), decisiondomain.Result{RequestID: "no-ad", Reason: decisiondomain.ReasonTargetingMiss}); err != nil {
		t.Fatal(err)
	}
	service := NewTraceService(decisions, events, "sync")
	trace, err := service.Read(t.Context(), "no-ad")
	if err != nil || trace.Decision == nil || trace.Decision.Matched || trace.Settlement != nil || len(trace.Events) != 0 {
		t.Fatalf("trace=%+v err=%v", trace, err)
	}
	if _, err := service.Read(t.Context(), "unknown"); !errors.Is(err, ErrTraceNotFound) {
		t.Fatal(err)
	}
}
func TestTraceUsesRetainedReceiptWithoutExposingSecrets(t *testing.T) {
	saved := decisiondomain.Result{RequestID: "r", UserID: "u", Matched: true, ReservationToken: "secret-reservation", RequestFingerprint: "secret-fingerprint", Pricing: decisiondomain.Pricing{Mode: "first_price", PriceFen: 5, AdvertiserName: "原广告主"}}
	state := domain.RequestState{Known: true, SavedDecision: &saved, Settlement: &domain.TraceSettlement{Status: "SETTLED"}, Events: []domain.TraceEvent{{EventID: "event", Status: "PUBLISHED"}}}
	trace, err := NewTraceService(emptyInspector{}, traceState{state: state}, "kafka").Read(t.Context(), "r")
	if err != nil {
		t.Fatal(err)
	}
	if trace.Decision == nil || trace.Decision.Pricing.PriceFen != 5 || trace.Events[0].ProcessedAt != nil {
		t.Fatalf("trace=%+v", trace)
	}
	payload, _ := json.Marshal(trace)
	if strings.Contains(string(payload), "secret-") || strings.Contains(string(payload), "reservationToken") {
		t.Fatal("private execution data leaked")
	}
}
func TestTraceShowsPendingExecutionWithoutInventingDecision(t *testing.T) {
	state := domain.RequestState{Known: true, Execution: &domain.TraceExecution{Status: "RUNNING"}}
	trace, err := NewTraceService(emptyInspector{}, traceState{state: state}, "kafka").Read(t.Context(), "r")
	if err != nil || trace.Decision != nil || trace.Execution.Status != "RUNNING" || trace.Events == nil {
		t.Fatalf("%+v %v", trace, err)
	}
}
func TestTraceDoesNotConsumeOrSettlePendingMemorySubmission(t *testing.T) {
	events := eventmemory.NewStore()
	decision := decisiondomain.Result{RequestID: "r", Matched: true, ReservationToken: "token"}
	_, err := events.PrepareImpression(t.Context(), domain.Event{EventID: "e", RequestID: "r", CampaignID: "c", Type: domain.Impression}, decision, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	events.ObserveSettlementAttempt(t.Context(), "e", errors.New("temporary Redis failure"))
	service := NewTraceService(emptyInspector{}, events, "sync")
	for range 2 {
		trace, err := service.Read(t.Context(), "r")
		if err != nil || trace.Settlement.Status != "UNCONFIRMED" || trace.Settlement.FailedAttempts != 1 || trace.Events[0].ProcessedAt != nil {
			t.Fatalf("%+v %v", trace, err)
		}
	}
	metrics, err := events.Metrics(t.Context(), "c")
	if err != nil || metrics.Impressions != 0 {
		t.Fatal("inspection changed metrics")
	}
}
