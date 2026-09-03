package application

import (
	"context"
	"time"

	"github.com/zhanghaiyang/adflow/internal/event/domain"
)

type Service struct {
	store domain.OperationsStore
	lag   domain.KafkaLagReader
	now   func() time.Time
}

func NewService(store domain.OperationsStore, lag domain.KafkaLagReader) *Service {
	return &Service{store: store, lag: lag, now: time.Now}
}

func (s *Service) Outbox(ctx context.Context, filter domain.OutboxFilter) ([]domain.OutboxRecord, domain.OutboxStats, error) {
	if s.store == nil {
		return []domain.OutboxRecord{}, domain.OutboxStats{}, nil
	}
	stats, err := s.store.Stats(ctx)
	if err != nil {
		return nil, domain.OutboxStats{}, err
	}
	items, err := s.store.ListOutbox(ctx, filter)
	return items, stats, err
}

func (s *Service) KafkaLag() []domain.KafkaPartitionLag {
	if s.lag == nil {
		return []domain.KafkaPartitionLag{}
	}
	return s.lag.KafkaLagSnapshot()
}

func (s *Service) ReplayDeadLetter(ctx context.Context, eventID string) error {
	if s.store == nil {
		return domain.ErrOutboxEntryNotFound
	}
	return s.store.ReplayDeadLetter(ctx, eventID, s.now().UTC())
}
