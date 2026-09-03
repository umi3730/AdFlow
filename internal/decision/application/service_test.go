package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zhanghaiyang/adflow/internal/decision/adapter/memory"
	"github.com/zhanghaiyang/adflow/internal/decision/domain"
)

type failingCandidates struct{}

func (failingCandidates) ActiveCandidates(context.Context, string, time.Time) ([]domain.Candidate, error) {
	return nil, errors.New("dependency unavailable")
}

type candidateProvider struct {
	candidates []domain.Candidate
}

type cleanupGate struct {
	frequencyReleased bool
	budgetReleased    bool
	cleanupCanceled   bool
}

func (g *cleanupGate) ReserveFrequency(context.Context, string, string, string, uint32, time.Time, time.Duration) (string, bool, error) {
	return "frequency-token", true, nil
}

func (g *cleanupGate) ReleaseFrequency(ctx context.Context, _ string) error {
	g.frequencyReleased = true
	g.cleanupCanceled = g.cleanupCanceled || ctx.Err() != nil
	return nil
}

func (*cleanupGate) ConfirmFrequency(context.Context, string, time.Time) error { return nil }

func (g *cleanupGate) ReserveBudget(context.Context, string, int64, int64, string, time.Time, time.Duration) (string, bool, error) {
	return "budget-token", true, nil
}

func (g *cleanupGate) ReleaseBudget(ctx context.Context, _ string) error {
	g.budgetReleased = true
	g.cleanupCanceled = g.cleanupCanceled || ctx.Err() != nil
	return nil
}

func (*cleanupGate) ConfirmBudget(context.Context, string, time.Time) error { return nil }

type cancelingDecisionStore struct{ cancel context.CancelFunc }

func (cancelingDecisionStore) FindDecision(context.Context, string) (domain.Result, bool, error) {
	return domain.Result{}, false, nil
}

func (s cancelingDecisionStore) SaveDecision(ctx context.Context, _ domain.Result) error {
	s.cancel()
	return ctx.Err()
}

func (p candidateProvider) ActiveCandidates(context.Context, string, time.Time) ([]domain.Candidate, error) {
	return append([]domain.Candidate(nil), p.candidates...), nil
}

func TestDecideMatchesAndIsIdempotent(t *testing.T) {
	runtime := memory.NewRuntime()
	_ = runtime.PutProfile(context.Background(), domain.NewProfile("user-1", []string{"anime"}, map[string]string{"device": "android"}))
	provider := candidateProvider{candidates: []domain.Candidate{{
		CampaignID: "campaign-1", CreativeIDs: []string{"creative-1"},
		Targeting:      domain.TargetingRule{All: []domain.Condition{{Tag: "anime"}}},
		DailyBudgetFen: 1000, ImpressionCostFen: 100, FrequencyLimit: 1,
	}}}
	service := NewService(provider, runtime, runtime, runtime, runtime)
	request := domain.Request{RequestID: "request-1", UserID: "user-1", SlotID: "slot-1", Now: time.Now().UTC()}
	first, err := service.Decide(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Decide(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Matched || first != second {
		t.Fatalf("unexpected results: first=%+v second=%+v", first, second)
	}

	capped, err := service.Decide(context.Background(), domain.Request{RequestID: "request-2", UserID: "user-1", SlotID: "slot-1", Now: request.Now})
	if err != nil {
		t.Fatal(err)
	}
	if capped.Matched || capped.Reason != domain.ReasonFrequencyCapped {
		t.Fatalf("expected frequency cap, got %+v", capped)
	}
}

func TestDecideReturnsNoAdForMissingProfile(t *testing.T) {
	runtime := memory.NewRuntime()
	service := NewService(candidateProvider{}, runtime, runtime, runtime, runtime)
	result, err := service.Decide(context.Background(), domain.Request{RequestID: "request-1", UserID: "missing", SlotID: "slot"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Matched || result.Reason != domain.ReasonProfileNotFound {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestDecideFailsClosedWhenCandidateDependencyIsUnavailable(t *testing.T) {
	runtime := memory.NewRuntime()
	_ = runtime.PutProfile(context.Background(), domain.NewProfile("user-1", []string{"anime"}, nil))
	service := NewService(failingCandidates{}, runtime, runtime, runtime, runtime)
	result, err := service.Decide(context.Background(), domain.Request{RequestID: "request-1", UserID: "user-1", SlotID: "slot"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Matched || result.Reason != domain.ReasonDependencyUnavailable {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestDecideReleasesReservationsAfterRequestCancellation(t *testing.T) {
	profiles := memory.NewRuntime()
	_ = profiles.PutProfile(t.Context(), domain.NewProfile("user-1", []string{"anime"}, nil))
	provider := candidateProvider{candidates: []domain.Candidate{{
		CampaignID: "campaign-1", CreativeIDs: []string{"creative-1"},
		Targeting:      domain.TargetingRule{All: []domain.Condition{{Tag: "anime"}}},
		DailyBudgetFen: 1000, ImpressionCostFen: 100, FrequencyLimit: 3,
	}}}
	ctx, cancel := context.WithCancel(t.Context())
	gates := &cleanupGate{}
	service := NewService(provider, profiles, gates, gates, cancelingDecisionStore{cancel: cancel})
	if _, err := service.Decide(ctx, domain.Request{RequestID: "request-canceled", UserID: "user-1", SlotID: "slot-1"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Decide() error=%v", err)
	}
	if !gates.frequencyReleased || !gates.budgetReleased || gates.cleanupCanceled {
		t.Fatalf("cleanup state: %+v", gates)
	}
}
