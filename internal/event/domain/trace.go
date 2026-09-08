package domain

import (
	"context"
	decisiondomain "github.com/zhanghaiyang/adflow/internal/decision/domain"
	"time"
)

const TraceEventLimit = 100

type TraceSettlement struct {
	Status         string     `json:"status"`
	FailedAttempts int        `json:"failedAttempts"`
	AcceptedAt     time.Time  `json:"acceptedAt"`
	SettledAt      *time.Time `json:"settledAt,omitempty"`
	NextAttemptAt  *time.Time `json:"nextAttemptAt,omitempty"`
	LockedUntil    *time.Time `json:"lockedUntil,omitempty"`
	LastError      string     `json:"lastError,omitempty"`
}
type TraceEvent struct {
	EventID        string     `json:"eventId"`
	Type           Type       `json:"type"`
	ValueFen       int64      `json:"valueFen,omitempty"`
	OccurredAt     time.Time  `json:"occurredAt"`
	AcceptedAt     time.Time  `json:"acceptedAt"`
	Status         string     `json:"status"`
	FailedAttempts int        `json:"failedAttempts"`
	LastError      string     `json:"lastError,omitempty"`
	NextAttemptAt  *time.Time `json:"nextAttemptAt,omitempty"`
	PublishedAt    *time.Time `json:"publishedAt,omitempty"`
	ProcessedAt    *time.Time `json:"processedAt,omitempty"`
}
type TraceExecution struct {
	Status     string     `json:"status"`
	LeaseUntil *time.Time `json:"leaseUntil,omitempty"`
}

// SavedDecision is internal; inspection never exposes reservation tokens.
type RequestState struct {
	ObservedAt    time.Time
	Known         bool
	SavedDecision *decisiondomain.Result
	Execution     *TraceExecution
	Settlement    *TraceSettlement
	Events        []TraceEvent
	Truncated     bool
}
type RequestStateReader interface {
	ReadRequestState(context.Context, string) (RequestState, error)
}
type SyncSettlementObserver interface {
	ObserveSettlementAttempt(context.Context, string, error)
}
