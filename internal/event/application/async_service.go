package application

import (
	"context"
	"strings"
	"time"

	"github.com/zhanghaiyang/adflow/internal/event/domain"
)

type AsyncService struct {
	processor *Service
	decisions domain.DecisionFinder
	outbox    domain.Enqueuer
	confirmer domain.ReservationConfirmer
	now       func() time.Time
}

func NewAsyncService(processor *Service, decisions domain.DecisionFinder, outbox domain.Enqueuer, confirmer domain.ReservationConfirmer) *AsyncService {
	return &AsyncService{processor: processor, decisions: decisions, outbox: outbox, confirmer: confirmer, now: time.Now}
}

func (s *AsyncService) Record(ctx context.Context, event domain.Event) (bool, error) {
	event.EventID = strings.TrimSpace(event.EventID)
	event.RequestID = strings.TrimSpace(event.RequestID)
	if event.EventID == "" || event.RequestID == "" || (event.Type != domain.Impression && event.Type != domain.Click && event.Type != domain.Conversion) || event.ValueFen < 0 {
		return false, domain.ErrInvalidEvent
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = s.now().UTC()
	}
	decision, found, err := s.decisions.FindDecision(ctx, event.RequestID)
	if err != nil {
		return false, err
	}
	if !found || !decision.Matched || decision.CampaignID != event.CampaignID || decision.CreativeID != event.CreativeID {
		return false, domain.ErrDecisionNotFound
	}
	if !decision.ExpiresAt.IsZero() && event.OccurredAt.After(decision.ExpiresAt) {
		return false, domain.ErrDecisionExpired
	}
	created, err := s.outbox.Enqueue(ctx, event)
	if err != nil || event.Type != domain.Impression || s.confirmer == nil {
		return created, err
	}
	// Confirm even when Enqueue reports an existing event. If a previous attempt
	// committed the Outbox transaction but failed during Redis settlement, the
	// duplicate request is the recovery path. Both confirmations are idempotent.
	if err := s.confirmer.ConfirmBudget(ctx, decision.ReservationToken, event.OccurredAt); err != nil {
		return false, err
	}
	if err := s.confirmer.ConfirmFrequency(ctx, decision.ReservationToken, event.OccurredAt); err != nil {
		return false, err
	}
	return created, nil
}

func (s *AsyncService) Metrics(ctx context.Context, campaignID string) (domain.Metrics, error) {
	return s.processor.Metrics(ctx, campaignID)
}
