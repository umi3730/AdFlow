package domain

import (
	"errors"
	"testing"
	"time"
)

func TestAttributionUsesServerAcceptanceAndOriginalIdentity(t *testing.T) {
	now := time.Now().UTC()
	receipt := ImpressionReceipt{Event: Event{RequestID: "r", CampaignID: "c", CreativeID: "cr", OccurredAt: now.Add(-time.Minute)}, AcceptedAt: now}
	event := Event{RequestID: "r", CampaignID: "c", CreativeID: "cr", OccurredAt: now.Add(DefaultAttributionWindow - time.Second)}
	if err := ValidateAttribution(event, receipt, event.OccurredAt); err != nil {
		t.Fatal(err)
	}
	event.CampaignID = "another"
	if err := ValidateAttribution(event, receipt, now); !errors.Is(err, ErrDecisionNotFound) {
		t.Fatal(err)
	}
	event.CampaignID = "c"
	event.OccurredAt = now.Add(-2 * time.Minute)
	if err := ValidateAttribution(event, receipt, now); !errors.Is(err, ErrInvalidEvent) {
		t.Fatal(err)
	}
}
