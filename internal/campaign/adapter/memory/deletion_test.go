package memory

import (
	"errors"
	"github.com/umi3730/adflow/internal/campaign/domain"
	"testing"
	"time"
)

func TestSoftDeletionPreservesHistoryAndRejectsStaleRevision(t *testing.T) {
	r := NewRepository()
	name, _ := domain.NewName("test campaign")
	slot, _ := domain.NewSlotID("banner")
	now := time.Now()
	period, _ := domain.NewDeliveryPeriod(now, now.Add(time.Hour))
	campaign := domain.NewCampaign("test", name, slot, period)
	if err := r.Create(t.Context(), campaign); err != nil {
		t.Fatal(err)
	}
	rule, _ := domain.NewTargetingRule([]domain.Condition{{Tag: "anime"}}, nil, nil)
	_ = campaign.Publish(rule, 100, 1, 3, now)
	if err := campaign.Delete(); !errors.Is(err, domain.ErrDeleteActive) {
		t.Fatal(err)
	}
	_ = campaign.Pause(now)
	if err := r.Save(t.Context(), campaign, 1); err != nil {
		t.Fatal(err)
	}
	stale := campaign.Clone()
	before := campaign.Revision()
	if err := campaign.Delete(); err != nil {
		t.Fatal(err)
	}
	if err := r.Save(t.Context(), campaign, before); err != nil {
		t.Fatal(err)
	}
	if _, err := r.FindByID(t.Context(), "test"); !errors.Is(err, domain.ErrCampaignNotFound) {
		t.Fatal(err)
	}
	listed, _ := r.List(t.Context(), domain.ListFilter{Limit: 100})
	if len(listed) != 0 || r.campaigns["test"].ActiveVersion() == nil {
		t.Fatal("deleted plan visible or history removed")
	}
	_ = stale.Resume(now)
	if err := r.Save(t.Context(), stale, before); !errors.Is(err, domain.ErrConcurrentMutation) {
		t.Fatal("stale save resurrected deleted campaign", err)
	}
}

func TestCreativeDeletionRequiresDisabledAndIsHidden(t *testing.T) {
	r := NewRepository()
	creative, _ := domain.NewCreative("creative", "campaign", "creative title", "", "https://example.com/a.png", "https://example.com")
	_ = r.CreateCreative(t.Context(), creative)
	if err := creative.Delete(); !errors.Is(err, domain.ErrDeleteActive) {
		t.Fatal(err)
	}
	_ = creative.Disable()
	_ = creative.Delete()
	if err := r.SaveCreative(t.Context(), creative, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := r.FindCreativeByID(t.Context(), "creative"); !errors.Is(err, domain.ErrCreativeNotFound) {
		t.Fatal(err)
	}
	listed, _ := r.ListCreativesByCampaign(t.Context(), "campaign")
	active, _ := r.ListActiveCreativeIDsByCampaigns(t.Context(), []string{"campaign"})
	if len(listed) != 0 || len(active["campaign"]) != 0 || r.creatives["creative"] == nil {
		t.Fatal("deleted creative visibility/history error")
	}
}
