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
	now       func() time.Time
}

func NewAsyncService(processor *Service, decisions domain.DecisionFinder, outbox domain.Enqueuer) *AsyncService {
	return &AsyncService{processor: processor, decisions: decisions, outbox: outbox, now: time.Now}
}

func (s *AsyncService) Record(ctx context.Context, event domain.Event) (bool, error) {
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
	return s.outbox.Enqueue(ctx, event)
}

func (s *AsyncService) Metrics(ctx context.Context, campaignID string) (domain.Metrics, error) {
	return s.processor.Metrics(ctx, campaignID)
}
