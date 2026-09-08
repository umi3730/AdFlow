package application

import (
	"context"
	"sort"
	"time"

	"github.com/umi3730/adflow/internal/decision/domain"
)

type CandidateExplanation struct {
	CampaignID       string                    `json:"campaignId"`
	TargetingMatched bool                      `json:"targetingMatched"`
	HasCreative      bool                      `json:"hasCreative"`
	Failures         []domain.ConditionFailure `json:"failures"`
}

type TargetingExplanation struct {
	Profile    domain.Profile
	Candidates []CandidateExplanation
	CheckedAt  time.Time
}

type ExplanationService struct {
	profiles   domain.ProfileStore
	candidates domain.CandidateProvider
}

func NewExplanationService(profiles domain.ProfileStore, candidates domain.CandidateProvider) *ExplanationService {
	return &ExplanationService{profiles: profiles, candidates: candidates}
}

func (s *ExplanationService) Explain(ctx context.Context, userID, slotID string) (TargetingExplanation, error) {
	profile, err := s.profiles.FindProfile(ctx, userID)
	if err != nil {
		return TargetingExplanation{}, err
	}
	now := time.Now().UTC()
	candidates, err := s.candidates.ActiveCandidates(ctx, slotID, now)
	if err != nil {
		return TargetingExplanation{}, err
	}
	result := TargetingExplanation{Profile: profile, Candidates: make([]CandidateExplanation, 0, len(candidates)), CheckedAt: now}
	evaluator := domain.Evaluator{}
	for _, candidate := range candidates {
		result.Candidates = append(result.Candidates, CandidateExplanation{
			CampaignID: candidate.CampaignID, TargetingMatched: evaluator.Match(profile, candidate.Targeting),
			HasCreative: len(candidate.CreativeIDs) > 0, Failures: evaluator.Explain(profile, candidate.Targeting),
		})
	}
	sort.Slice(result.Candidates, func(i, j int) bool { return result.Candidates[i].CampaignID < result.Candidates[j].CampaignID })
	return result, nil
}
