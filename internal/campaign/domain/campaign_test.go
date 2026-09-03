package domain

import (
	"errors"
	"testing"
	"time"
)

func validCampaign(t *testing.T) *Campaign {
	t.Helper()
	name, _ := NewName("Strategy Game Launch")
	slot, _ := NewSlotID("game-home-banner")
	period, _ := NewDeliveryPeriod(time.Now(), time.Now().Add(24*time.Hour))
	return NewCampaign("cmp-1", name, slot, period)
}

func validRule(t *testing.T) TargetingRule {
	t.Helper()
	rule, err := NewTargetingRule([]Condition{{Tag: "anime"}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return rule
}

func TestCampaignLifecycle(t *testing.T) {
	campaign := validCampaign(t)
	now := time.Now()
	if err := campaign.Publish(validRule(t), 10_000, 100, 3, now); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if campaign.Status() != StatusActive || campaign.ActiveVersion().Number() != 1 {
		t.Fatalf("unexpected campaign after publish: status=%s version=%v", campaign.Status(), campaign.ActiveVersion())
	}
	if err := campaign.Pause(now); err != nil {
		t.Fatalf("Pause() error = %v", err)
	}
	if err := campaign.Resume(now); err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	if campaign.Status() != StatusActive {
		t.Fatalf("status = %s", campaign.Status())
	}
}

func TestCampaignRejectsInvalidTransition(t *testing.T) {
	campaign := validCampaign(t)
	if err := campaign.Pause(time.Now()); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("Pause() error = %v", err)
	}
}

func TestOnlyDraftCampaignCanBeEdited(t *testing.T) {
	campaign := validCampaign(t)
	name, _ := NewName("Updated Campaign")
	slot, _ := NewSlotID("updated-slot")
	period, _ := NewDeliveryPeriod(time.Now(), time.Now().Add(48*time.Hour))
	if err := campaign.UpdateDraft(name, slot, period); err != nil {
		t.Fatal(err)
	}
	if campaign.Name() != name || campaign.Revision() != 2 {
		t.Fatalf("campaign was not updated: %+v", campaign)
	}
	if err := campaign.Publish(validRule(t), 1000, 100, 3, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := campaign.UpdateDraft(name, slot, period); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("UpdateDraft() error = %v", err)
	}
}

func TestTargetingConditionMustChooseTagOrField(t *testing.T) {
	_, err := NewTargetingRule([]Condition{{Tag: "anime", Field: "device", Op: "eq", Value: "android"}}, nil, nil)
	if !errors.Is(err, ErrInvalidTargeting) {
		t.Fatalf("NewTargetingRule() error = %v", err)
	}
}
