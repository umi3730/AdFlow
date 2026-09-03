package domain

import (
	"context"
	"errors"
	"time"

	decisiondomain "github.com/zhanghaiyang/adflow/internal/decision/domain"
)

var (
	ErrInvalidEvent        = errors.New("ad event is invalid")
	ErrDecisionNotFound    = errors.New("matched decision not found")
	ErrDecisionExpired     = errors.New("advertising decision has expired")
	ErrImpressionRequired  = errors.New("impression must be recorded first")
	ErrOutboxEntryNotFound = errors.New("outbox entry not found")
	ErrOutboxNotDeadLetter = errors.New("outbox entry is not dead-lettered")
)

type Type string

const (
	Impression Type = "impression"
	Click      Type = "click"
	Conversion Type = "conversion"
)

type Event struct {
	EventID    string    `json:"eventId"`
	RequestID  string    `json:"requestId"`
	CampaignID string    `json:"campaignId"`
	CreativeID string    `json:"creativeId"`
	Type       Type      `json:"type"`
	ValueFen   int64     `json:"valueFen,omitempty"`
	OccurredAt time.Time `json:"occurredAt"`
}

type Metrics struct {
	CampaignID  string `json:"campaignId"`
	Impressions uint64 `json:"impressions"`
	Clicks      uint64 `json:"clicks"`
	Conversions uint64 `json:"conversions"`
	ValueFen    int64  `json:"valueFen"`
}

type Store interface {
	Record(context.Context, Event) (bool, error)
	HasImpression(context.Context, string) (bool, error)
	Metrics(context.Context, string) (Metrics, error)
}

type BatchStore interface {
	RecordBatch(context.Context, []Event) ([]bool, error)
	HasImpressions(context.Context, []string) (map[string]bool, error)
}

type DecisionFinder interface {
	FindDecision(context.Context, string) (decisiondomain.Result, bool, error)
}

type BatchDecisionFinder interface {
	FindDecisions(context.Context, []string) (map[string]decisiondomain.Result, error)
}

type ReservationConfirmer interface {
	ConfirmFrequency(context.Context, string, time.Time) error
	ConfirmBudget(context.Context, string, time.Time) error
}

type Publisher interface {
	PublishEvent(context.Context, Event) error
}

type BatchPublisher interface {
	PublishEvents(context.Context, []Event) error
}

type DeadLetterPublisher interface {
	PublishDeadLetter(context.Context, Event, string, int, time.Time) error
}

type Enqueuer interface {
	Enqueue(context.Context, Event) (bool, error)
}

type OutboxEntry struct {
	Event    Event
	Attempts int
}

type Outbox interface {
	Enqueuer
	ClaimBatch(context.Context, int, time.Time, time.Duration) ([]OutboxEntry, error)
	MarkPublished(context.Context, string, time.Time) error
	MarkFailed(context.Context, string, string, time.Time) error
	MarkDeadLetter(context.Context, string, string, time.Time) error
	Stats(context.Context) (OutboxStats, error)
}

type PublishedBatchMarker interface {
	MarkPublishedBatch(context.Context, []string, time.Time) error
}

type OutboxStats struct {
	Pending      int64 `json:"pending"`
	Processing   int64 `json:"processing"`
	Published    int64 `json:"published"`
	DeadLettered int64 `json:"deadLettered"`
}

type OutboxRecord struct {
	Event          Event      `json:"event"`
	Status         string     `json:"status"`
	Attempts       int        `json:"attempts"`
	NextAttemptAt  time.Time  `json:"nextAttemptAt"`
	LockedBy       string     `json:"lockedBy,omitempty"`
	LockedUntil    *time.Time `json:"lockedUntil,omitempty"`
	PublishedAt    *time.Time `json:"publishedAt,omitempty"`
	DeadLetteredAt *time.Time `json:"deadLetteredAt,omitempty"`
	LastError      string     `json:"lastError,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
}

type OutboxFilter struct {
	Status string
	Limit  int
	Offset int
}

type OperationsStore interface {
	Stats(context.Context) (OutboxStats, error)
	ListOutbox(context.Context, OutboxFilter) ([]OutboxRecord, error)
	ReplayDeadLetter(context.Context, string, time.Time) error
}

type KafkaPartitionLag struct {
	Topic     string `json:"topic"`
	Partition int32  `json:"partition"`
	Lag       int64  `json:"lag"`
}

type KafkaLagReader interface {
	KafkaLagSnapshot() []KafkaPartitionLag
}
