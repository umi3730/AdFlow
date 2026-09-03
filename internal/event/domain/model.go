package domain

import (
	"context"
	"errors"
	"time"

	decisiondomain "github.com/zhanghaiyang/adflow/internal/decision/domain"
)

var (
	ErrInvalidEvent       = errors.New("ad event is invalid")
	ErrDecisionNotFound   = errors.New("matched decision not found")
	ErrImpressionRequired = errors.New("impression must be recorded first")
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

type DecisionFinder interface {
	FindDecision(context.Context, string) (decisiondomain.Result, bool, error)
}

type ReservationConfirmer interface {
	ConfirmFrequency(context.Context, string, time.Time) error
	ConfirmBudget(context.Context, string, time.Time) error
}

type Publisher interface {
	PublishEvent(context.Context, Event) error
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

type OutboxStats struct {
	Pending      int64
	Processing   int64
	Published    int64
	DeadLettered int64
}
