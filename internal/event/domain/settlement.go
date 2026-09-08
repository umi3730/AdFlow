package domain

import (
	"context"
	"errors"
	decisiondomain "github.com/umi3730/adflow/internal/decision/domain"
	"time"
)

var ErrEventConflict = errors.New("event ID or impression request is already bound to another event")
var ErrSettlementPending = errors.New("event has no confirmed settlement")
var ErrSettlementLeaseLost = errors.New("settlement lease lost")

// SameEventIdentity ignores occurrence time: an omitted timestamp is assigned
// once at ingress, and a retry must retain the first accepted timestamp.
func SameEventIdentity(a, b Event) bool {
	return a.EventID == b.EventID && a.RequestID == b.RequestID && a.CampaignID == b.CampaignID && a.CreativeID == b.CreativeID && a.Type == b.Type && a.ValueFen == b.ValueFen
}

type SettlementIngress interface {
	FindAcceptedEvent(context.Context, string) (Event, bool, error)
	EnqueueForSettlement(context.Context, Event, decisiondomain.Result, time.Time) (bool, error)
}

type SettlementEntry struct {
	Settlement decisiondomain.Settlement
	Attempts   int
	Owner      string
}

type SettlementQueue interface {
	ClaimSettlements(context.Context, int, time.Duration) ([]SettlementEntry, error)
	CompleteSettlement(context.Context, SettlementEntry) error
	FailSettlement(context.Context, SettlementEntry, string, time.Duration, bool) error
	// Release only unfinished entries still owned by this claim. Preserve settled,
	// quarantined, explicitly rescheduled, or newly claimed entries.
	ReleaseSettlements(context.Context, []SettlementEntry) error
}

type SettlementProof interface {
	VerifySettledEvents(context.Context, []Event) error
}
