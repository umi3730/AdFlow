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
