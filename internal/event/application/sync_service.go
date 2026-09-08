package application

import (
	"context"
	"errors"
	decisiondomain "github.com/zhanghaiyang/adflow/internal/decision/domain"
	"github.com/zhanghaiyang/adflow/internal/event/domain"
)

func (s *Service) recordSync(ctx context.Context, event domain.Event) (bool, error) {
	store, ok := s.store.(domain.SyncStore)
	if !ok {
		return false, errors.New("synchronous events require a receipt-aware store")
	}
	accepted, exists, err := store.FindAcceptedEvent(ctx, event.EventID)
	if err != nil {
		return false, err
	}
	if exists {
		if !domain.SameEventIdentity(accepted, event) {
			return false, domain.ErrEventConflict
		}
		event = accepted
		if event.Type != domain.Impression {
			return false, nil
		}
	}
	now := s.now().UTC()
	if event.OccurredAt.IsZero() {
		event.OccurredAt = now
	}
	if event.Type != domain.Impression {
		receipt, found, err := store.FindImpressionReceipt(ctx, event.RequestID)
		if err != nil {
			return false, err
		}
		if !found {
			return false, domain.ErrImpressionRequired
		}
		if err := domain.ValidateAttribution(event, receipt, now); err != nil {
			return false, err
		}
		return store.Record(ctx, event)
	}
	submission, found, err := store.FindImpressionSubmission(ctx, event.EventID)
	if err != nil {
		return false, err
	}
	if !found {
		decision, found, err := s.decisions.FindDecision(ctx, event.RequestID)
		if err != nil {
			return false, err
		}
		if !found || !decision.Matched || decision.CampaignID != event.CampaignID || decision.CreativeID != event.CreativeID {
			return false, domain.ErrDecisionNotFound
		}
		if !decision.ExpiresAt.IsZero() && (!now.Before(decision.ExpiresAt) || event.OccurredAt.After(decision.ExpiresAt)) {
			return false, domain.ErrDecisionExpired
		}
		submission, err = store.PrepareImpression(ctx, event, decision, now)
		if err != nil {
			return false, err
		}
	}
	if !domain.SameEventIdentity(event, submission.Event) {
		return false, domain.ErrEventConflict
	}
	if submission.Processed {
		return false, nil
	}
	settleErr := s.confirmer.SettleImpression(ctx, decisiondomain.Settlement{EventID: submission.Event.EventID, Decision: submission.Decision})
	if observer, ok := store.(domain.SyncSettlementObserver); ok {
		observer.ObserveSettlementAttempt(ctx, submission.Event.EventID, settleErr)
	}
	if settleErr != nil {
		return false, settleErr
	}
	return store.CommitImpression(ctx, submission.Event.EventID)
}
