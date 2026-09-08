package application

import (
	"context"
	"time"

	"github.com/umi3730/adflow/internal/event/domain"
)

type Service struct {
	store domain.OperationsStore
	lag   domain.KafkaLagReader
	now   func() time.Time
}

func NewService(store domain.OperationsStore, lag domain.KafkaLagReader) *Service {
	return &Service{store: store, lag: lag, now: time.Now}
}

type RuntimeMode struct {
	EventTransport string `json:"eventTransport"`
	OutboxEnabled  bool   `json:"outboxEnabled"`
}

// This reports wiring, not Kafka connectivity or consumer health.
func (s *Service) Mode() RuntimeMode {
	if s.store != nil {
		return RuntimeMode{EventTransport: "kafka", OutboxEnabled: true}
	}
	return RuntimeMode{EventTransport: "sync", OutboxEnabled: false}
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
