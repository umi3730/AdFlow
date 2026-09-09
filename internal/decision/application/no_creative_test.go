package application

import (
	"testing"
	"time"

	"github.com/umi3730/adflow/internal/decision/adapter/memory"
	"github.com/umi3730/adflow/internal/decision/domain"
)

func TestMissingCreativeReasonAndReplay(t *testing.T) {
	runtime := memory.NewRuntime()
	_ = runtime.PutProfile(t.Context(), domain.NewProfile("user", []string{"auction"}, nil))
	candidate := bidCandidate("plan", "advertiser", 5)
	candidate.CreativeIDs = nil
	candidates := []domain.Candidate{candidate}
	service := NewService(candidateProvider{candidates}, runtime, runtime, runtime, runtime)
	request := domain.Request{RequestID: "missing-creative", UserID: "user", SlotID: "slot", Now: time.Now().UTC()}
	result, err := service.Decide(t.Context(), request)
	if err != nil || result.Matched || result.Reason != domain.ReasonNoCreative {
		t.Fatalf("result=%+v error=%v", result, err)
	}
	candidates[0].CreativeIDs = []string{"creative"}
	replayed, err := service.Decide(t.Context(), request)
	if err != nil || replayed != result {
		t.Fatalf("historical reason changed: %+v %v", replayed, err)
	}
	request.RequestID = "creative-added"
	next, err := service.Decide(t.Context(), request)
	if err != nil || !next.Matched {
		t.Fatalf("new decision did not recover: %+v %v", next, err)
	}
}

func TestEmptyRankingDistinguishesTargetingAndPeriod(t *testing.T) {
	now := time.Now().UTC()
	profile := domain.NewProfile("user", []string{"other"}, nil)
	candidate := bidCandidate("plan", "advertiser", 5)
	candidate.CreativeIDs = nil
	if got := unavailableCandidateReason([]domain.Candidate{candidate}, profile, now); got != domain.ReasonTargetingMiss {
		t.Fatal(got)
	}
	candidate.EndAt = now
	if got := unavailableCandidateReason([]domain.Candidate{candidate}, profile, now); got != domain.ReasonNoCandidate {
		t.Fatal(got)
	}
}
