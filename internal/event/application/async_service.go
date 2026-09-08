package application

import (
	"context"
	decisiondomain "github.com/umi3730/adflow/internal/decision/domain"
	"strings"
	"time"

	"github.com/umi3730/adflow/internal/event/domain"
)

type AsyncService struct {
	processor *Service
	decisions domain.DecisionFinder
	outbox    domain.SettlementIngress
	now       func() time.Time
}

func NewAsyncService(processor *Service, decisions domain.DecisionFinder, outbox domain.SettlementIngress) *AsyncService {
	return &AsyncService{processor: processor, decisions: decisions, outbox: outbox, now: time.Now}
}

func (s *AsyncService) Record(ctx context.Context, event domain.Event) (bool, error) {
	event.EventID = strings.TrimSpace(event.EventID)
	event.RequestID = strings.TrimSpace(event.RequestID)
	if event.EventID == "" || event.RequestID == "" || (event.Type != domain.Impression && event.Type != domain.Click && event.Type != domain.Conversion) || event.ValueFen < 0 {
		return false, domain.ErrInvalidEvent
	}
	accepted, exists, err := s.outbox.FindAcceptedEvent(ctx, event.EventID)
	if err != nil {
		return false, err
	}
	if exists {
		if !domain.SameEventIdentity(accepted, event) {
			return false, domain.ErrEventConflict
		}
		return false, nil
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = s.now().UTC()
	}
	if event.Type != domain.Impression {
		reader, ok := s.outbox.(domain.ImpressionReader)
		if !ok {
			return false, domain.ErrImpressionRequired
		}
		receipt, found, err := reader.FindImpressionReceipt(ctx, event.RequestID)
		if err != nil {
			return false, err
		}
		if !found {
			return false, domain.ErrImpressionRequired
		}
		if err := domain.ValidateAttribution(event, receipt, s.now().UTC()); err != nil {
			return false, err
		}
		return s.outbox.EnqueueForSettlement(ctx, event, decisiondomain.Result{}, s.now().UTC())
	}
	decision, found, err := s.decisions.FindDecision(ctx, event.RequestID)
	if err != nil {
		return false, err
	}
	if !found || !decision.Matched || decision.CampaignID != event.CampaignID || decision.CreativeID != event.CreativeID {
		return false, domain.ErrDecisionNotFound
	}
	if !decision.ExpiresAt.IsZero() && !s.now().UTC().Before(decision.ExpiresAt) {
		return false, domain.ErrDecisionExpired
	}
	if !decision.ExpiresAt.IsZero() && event.OccurredAt.After(decision.ExpiresAt) {
		return false, domain.ErrDecisionExpired
	}
	return s.outbox.EnqueueForSettlement(ctx, event, decision, s.now().UTC())
}

func (s *AsyncService) Metrics(ctx context.Context, campaignID string) (domain.Metrics, error) {
	return s.processor.Metrics(ctx, campaignID)
}
