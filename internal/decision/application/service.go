package application

import (
	"context"
	"hash/fnv"
	"sort"
	"strings"
	"time"

	"github.com/zhanghaiyang/adflow/internal/decision/domain"
)

const reservationTTL = 30 * time.Second

type Service struct {
	candidates domain.CandidateProvider
	profiles   domain.ProfileStore
	frequency  domain.FrequencyGate
	budget     domain.BudgetGate
	decisions  domain.DecisionStore
	evaluator  domain.Evaluator
	now        func() time.Time
}

func NewService(candidates domain.CandidateProvider, profiles domain.ProfileStore, frequency domain.FrequencyGate, budget domain.BudgetGate, decisions domain.DecisionStore) *Service {
	return &Service{
		candidates: candidates, profiles: profiles, frequency: frequency, budget: budget,
		decisions: decisions, evaluator: domain.Evaluator{}, now: time.Now,
	}
}

func (s *Service) Decide(ctx context.Context, request domain.Request) (domain.Result, error) {
	request.RequestID = strings.TrimSpace(request.RequestID)
	request.UserID = strings.TrimSpace(request.UserID)
	request.SlotID = strings.TrimSpace(request.SlotID)
	if request.RequestID == "" || request.UserID == "" || request.SlotID == "" {
		return domain.Result{}, domain.ErrInvalidRequest
	}
	if existing, found, err := s.decisions.FindDecision(ctx, request.RequestID); err != nil {
		return domain.Result{}, err
	} else if found {
		return existing, nil
	}

	now := request.Now
	if now.IsZero() {
		now = s.now().UTC()
	}
	profile, err := s.profiles.FindProfile(ctx, request.UserID)
	if err != nil {
		if err == domain.ErrProfileNotFound {
			return s.noAd(ctx, request, domain.ReasonProfileNotFound)
		}
		return s.noAd(ctx, request, domain.ReasonDependencyUnavailable)
	}
	candidates, err := s.candidates.ActiveCandidates(ctx, request.SlotID, now)
	if err != nil {
		return s.noAd(ctx, request, domain.ReasonDependencyUnavailable)
	}
	if len(candidates) == 0 {
		return s.noAd(ctx, request, domain.ReasonNoCandidate)
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].CampaignID < candidates[j].CampaignID })

	reason := domain.ReasonTargetingMiss
	for _, candidate := range candidates {
		if len(candidate.CreativeIDs) == 0 || !s.evaluator.Match(profile, candidate.Targeting) {
			continue
		}
		frequencyToken, allowed, reserveErr := s.frequency.ReserveFrequency(
			ctx, request.UserID, candidate.CampaignID, request.RequestID, candidate.FrequencyLimit, now, reservationTTL,
		)
		if reserveErr != nil {
			return s.noAd(ctx, request, domain.ReasonDependencyUnavailable)
		}
		if !allowed {
			reason = domain.ReasonFrequencyCapped
			continue
		}
		budgetToken, allowed, reserveErr := s.budget.ReserveBudget(
			ctx, candidate.CampaignID, candidate.DailyBudgetFen, candidate.ImpressionCostFen, request.RequestID, now, reservationTTL,
		)
		if reserveErr != nil {
			_ = s.frequency.ReleaseFrequency(ctx, frequencyToken)
			return s.noAd(ctx, request, domain.ReasonDependencyUnavailable)
		}
		if !allowed {
			_ = s.frequency.ReleaseFrequency(ctx, frequencyToken)
			reason = domain.ReasonBudgetExhausted
			continue
		}

		result := domain.Result{
			RequestID: request.RequestID, UserID: request.UserID, SlotID: request.SlotID,
			Matched: true, CampaignID: candidate.CampaignID,
			CreativeID:       chooseCreative(request.RequestID, candidate.CreativeIDs),
			ReservationToken: budgetToken, ExpiresAt: now.Add(reservationTTL), Reason: domain.ReasonMatched,
		}
		if err := s.decisions.SaveDecision(ctx, result); err != nil {
			_ = s.frequency.ReleaseFrequency(ctx, frequencyToken)
			_ = s.budget.ReleaseBudget(ctx, budgetToken)
			return domain.Result{}, err
		}
		return result, nil
	}
	return s.noAd(ctx, request, reason)
}

func (s *Service) noAd(ctx context.Context, request domain.Request, reason domain.Reason) (domain.Result, error) {
	result := domain.Result{RequestID: request.RequestID, UserID: request.UserID, SlotID: request.SlotID, Matched: false, Reason: reason}
	if err := s.decisions.SaveDecision(ctx, result); err != nil {
		return domain.Result{}, err
	}
	return result, nil
}

func chooseCreative(requestID string, creativeIDs []string) string {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(requestID))
	return creativeIDs[int(hash.Sum32())%len(creativeIDs)]
}
