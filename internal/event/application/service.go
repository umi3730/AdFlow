package application

import (
	"context"
	decisiondomain "github.com/umi3730/adflow/internal/decision/domain"
	"strings"
	"time"

	"github.com/umi3730/adflow/internal/event/domain"
)

type Service struct {
	store     domain.Store
	decisions domain.DecisionFinder
	confirmer decisiondomain.ImpressionSettler
	now       func() time.Time
}

func NewService(store domain.Store, decisions domain.DecisionFinder, confirmer decisiondomain.ImpressionSettler) *Service {
	return &Service{store: store, decisions: decisions, confirmer: confirmer, now: time.Now}
}

func (s *Service) Record(ctx context.Context, event domain.Event) (bool, error) {
	event.EventID = strings.TrimSpace(event.EventID)
	event.RequestID = strings.TrimSpace(event.RequestID)
	if event.EventID == "" || event.RequestID == "" || (event.Type != domain.Impression && event.Type != domain.Click && event.Type != domain.Conversion) || event.ValueFen < 0 {
		return false, domain.ErrInvalidEvent
	}
	if s.confirmer != nil {
		return s.recordSync(ctx, event)
	}
	decision, found, err := s.decisions.FindDecision(ctx, event.RequestID)
	if err != nil {
		return false, err
	}
	if !found || !decision.Matched || decision.CampaignID != event.CampaignID || decision.CreativeID != event.CreativeID {
		return false, domain.ErrDecisionNotFound
	}
	// This is the trusted consumer path. Ingress validates attribution before
	// enqueue, and RecordSettledBatch verifies the durable accepted payload.
	if event.OccurredAt.IsZero() {
		event.OccurredAt = s.now().UTC()
	}
	if event.Type == domain.Impression && !decision.ExpiresAt.IsZero() && event.OccurredAt.After(decision.ExpiresAt) {
		return false, domain.ErrDecisionExpired
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
	return created, nil
}

func (s *Service) RecordBatch(ctx context.Context, events []domain.Event) error {
	if len(events) == 0 {
		return nil
	}
	if s.confirmer != nil {
		for _, event := range events {
			if _, err := s.Record(ctx, event); err != nil {
				return err
			}
		}
		return nil
	}
	batchStore, storeSupportsBatch := s.store.(domain.BatchStore)
	batchDecisions, decisionsSupportBatch := s.decisions.(domain.BatchDecisionFinder)
	if !storeSupportsBatch || !decisionsSupportBatch {
		for _, event := range events {
			if _, err := s.Record(ctx, event); err != nil {
				return err
			}
		}
		return nil
	}
	prepared := make([]domain.Event, len(events))
	requestIDs := make([]string, 0, len(events))
	impressionDependencies := make([]string, 0)
	for index, event := range events {
		event.EventID = strings.TrimSpace(event.EventID)
		event.RequestID = strings.TrimSpace(event.RequestID)
		if event.EventID == "" || event.RequestID == "" || (event.Type != domain.Impression && event.Type != domain.Click && event.Type != domain.Conversion) || event.ValueFen < 0 {
			return domain.ErrInvalidEvent
		}
		if event.OccurredAt.IsZero() {
			event.OccurredAt = s.now().UTC()
		}
		prepared[index] = event
		requestIDs = append(requestIDs, event.RequestID)
		if event.Type != domain.Impression {
			impressionDependencies = append(impressionDependencies, event.RequestID)
		}
	}
	decisions, err := batchDecisions.FindDecisions(ctx, requestIDs)
	if err != nil {
		return err
	}
	hasImpression, err := batchStore.HasImpressions(ctx, impressionDependencies)
	if err != nil {
		return err
	}
	seenImpression := make(map[string]bool)
	for _, event := range prepared {
		decision, found := decisions[event.RequestID]
		if !found || !decision.Matched || decision.CampaignID != event.CampaignID || decision.CreativeID != event.CreativeID {
			return domain.ErrDecisionNotFound
		}
		if event.Type == domain.Impression && !decision.ExpiresAt.IsZero() && event.OccurredAt.After(decision.ExpiresAt) {
			return domain.ErrDecisionExpired
		}
		if event.Type != domain.Impression && !seenImpression[event.RequestID] && !hasImpression[event.RequestID] {
			return domain.ErrImpressionRequired
		}
		if event.Type == domain.Impression {
			seenImpression[event.RequestID] = true
		}
	}
	if _, err := batchStore.RecordBatch(ctx, prepared); err != nil {
		return err
	}
	return nil
}

func (s *Service) Metrics(ctx context.Context, campaignID string) (domain.Metrics, error) {
	return s.store.Metrics(ctx, campaignID)
}
