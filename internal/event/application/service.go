package application

import (
	"context"
	"strings"
	"time"

	"github.com/zhanghaiyang/adflow/internal/event/domain"
)

type Service struct {
	store     domain.Store
	decisions domain.DecisionFinder
	confirmer domain.ReservationConfirmer
	now       func() time.Time
}

func NewService(store domain.Store, decisions domain.DecisionFinder, confirmer domain.ReservationConfirmer) *Service {
	return &Service{store: store, decisions: decisions, confirmer: confirmer, now: time.Now}
}

func (s *Service) Record(ctx context.Context, event domain.Event) (bool, error) {
	event.EventID = strings.TrimSpace(event.EventID)
	event.RequestID = strings.TrimSpace(event.RequestID)
	if event.EventID == "" || event.RequestID == "" || (event.Type != domain.Impression && event.Type != domain.Click && event.Type != domain.Conversion) || event.ValueFen < 0 {
		return false, domain.ErrInvalidEvent
	}
	decision, found, err := s.decisions.FindDecision(ctx, event.RequestID)
	if err != nil {
		return false, err
	}
	if !found || !decision.Matched || decision.CampaignID != event.CampaignID || decision.CreativeID != event.CreativeID {
		return false, domain.ErrDecisionNotFound
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = s.now().UTC()
	}
	if event.Type != domain.Impression {
		hasImpression, err := s.store.HasImpression(ctx, event.RequestID)
		if err != nil {
			return false, err
		}
		if !hasImpression {
			return false, domain.ErrImpressionRequired
		}
	}
	created, err := s.store.Record(ctx, event)
	if err != nil {
		return false, err
	}
	if event.Type == domain.Impression {
		if err := s.confirmer.ConfirmFrequency(ctx, decision.ReservationToken, event.OccurredAt); err != nil {
			return false, err
		}
		if err := s.confirmer.ConfirmBudget(ctx, decision.ReservationToken, event.OccurredAt); err != nil {
			return false, err
		}
	}
	return created, nil
}

func (s *Service) Metrics(ctx context.Context, campaignID string) (domain.Metrics, error) {
	return s.store.Metrics(ctx, campaignID)
}
