package domain

import (
	"testing"
	"time"
)

func TestLegacyTextComparisonCanBeReadButNotPublished(t *testing.T) {
	rule, err := NewTargetingRule([]Condition{{Field: "country", Op: "gte", Value: "156"}}, nil, nil)
	if err != nil {
		t.Fatalf("old rule must remain readable: %v", err)
	}
	if rule.ValidateForPublication() == nil {
		t.Fatal("country cannot be compared numerically")
	}
	for _, group := range []TargetingRule{{All: rule.All}, {Any: rule.All}, {None: rule.All}} {
		if group.ValidateForPublication() == nil {
			t.Fatal("group bypassed validation")
		}
	}
	name, _ := NewName("test campaign")
	slot, _ := NewSlotID("banner")
	period, _ := NewDeliveryPeriod(time.Now(), time.Now().Add(time.Hour))
	campaign := NewCampaign("id", name, slot, period)
	if campaign.Publish(rule, 100, 1, 1, time.Now()) == nil || campaign.Status() != StatusDraft {
		t.Fatal("invalid rule published")
	}
	version, _ := NewVersion(1, rule, 100, 1, 1, time.Now())
	legacy := Rehydrate("old", name, slot, period, StatusPaused, &version, 2)
	if legacy.Resume(time.Now()) == nil {
		t.Fatal("invalid legacy rule resumed")
	}
}
