package domain

import (
	"context"
	"errors"
	decisiondomain "github.com/zhanghaiyang/adflow/internal/decision/domain"
	"time"
)

const DefaultAttributionWindow = 7 * 24 * time.Hour

var ErrAttributionExpired = errors.New("event attribution window has expired")

type ImpressionReceipt struct {
	Event      Event
	AcceptedAt time.Time
}
type ImpressionReader interface {
	FindImpressionReceipt(context.Context, string) (ImpressionReceipt, bool, error)
}
type ImpressionSubmission struct {
	ImpressionReceipt
	Decision       decisiondomain.Result
	Processed      bool
	FailedAttempts int
	LastError      string
}

// Sync stores stage immutable identity before external settlement. Metrics
// become visible only after settlement succeeds; duplicates reuse that stage.
type SyncStore interface {
	Store
	ImpressionReader
	FindAcceptedEvent(context.Context, string) (Event, bool, error)
	FindImpressionSubmission(context.Context, string) (ImpressionSubmission, bool, error)
	PrepareImpression(context.Context, Event, decisiondomain.Result, time.Time) (ImpressionSubmission, error)
	CommitImpression(context.Context, string) (bool, error)
}

func ValidateAttribution(event Event, receipt ImpressionReceipt, now time.Time) error {
	if event.RequestID != receipt.Event.RequestID || event.CampaignID != receipt.Event.CampaignID || event.CreativeID != receipt.Event.CreativeID {
		return ErrDecisionNotFound
	}
	end := receipt.AcceptedAt.Add(DefaultAttributionWindow)
	if !now.Before(end) || event.OccurredAt.After(end) {
		return ErrAttributionExpired
	}
	if event.OccurredAt.Before(receipt.Event.OccurredAt) {
		return ErrInvalidEvent
	}
	return nil
}
