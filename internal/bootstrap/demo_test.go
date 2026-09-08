package bootstrap

import (
	"errors"
	"sync"
	"testing"
	"time"

	campaignmemory "github.com/zhanghaiyang/adflow/internal/campaign/adapter/memory"
	campaignapp "github.com/zhanghaiyang/adflow/internal/campaign/application"
	campaign "github.com/zhanghaiyang/adflow/internal/campaign/domain"
	candidates "github.com/zhanghaiyang/adflow/internal/decision/adapter/campaign"
	decisionmemory "github.com/zhanghaiyang/adflow/internal/decision/adapter/memory"
	decisionapp "github.com/zhanghaiyang/adflow/internal/decision/application"
	decision "github.com/zhanghaiyang/adflow/internal/decision/domain"
	eventmemory "github.com/zhanghaiyang/adflow/internal/event/adapter/memory"
	eventapp "github.com/zhanghaiyang/adflow/internal/event/application"
	event "github.com/zhanghaiyang/adflow/internal/event/domain"
)

func memoryDemo(t *testing.T) (*DemoInitializer, *campaignmemory.Repository, *decisionmemory.Runtime) {
	t.Helper()
	campaigns, profiles := campaignmemory.NewRepository(), decisionmemory.NewRuntime()
	initializer, err := NewDemoInitializer(DemoOptions{Campaigns: campaigns, Profiles: profiles})
	if err != nil {
		t.Fatal(err)
	}
	return initializer, campaigns, profiles
}

func TestDemoProvidesMatchedExcludedAndMissingTagScenarios(t *testing.T) {
	initializer, campaigns, runtime := memoryDemo(t)
	now := time.Now().UTC()
	result, err := initializer.Run(t.Context(), now)
	if err != nil || !result.CampaignsInstalled || !result.ProfilesInstalled {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	provider := candidates.NewProvider(campaigns, campaigns)
	engine := decisionapp.NewService(provider, runtime, runtime, runtime, runtime)
	events := eventapp.NewService(eventmemory.NewStore(), runtime, runtime)
	initialMetrics, err := events.Metrics(t.Context(), DemoCampaignID)
	if err != nil || initialMetrics.Impressions != 0 || initialMetrics.Clicks != 0 || initialMetrics.Conversions != 0 {
		t.Fatalf("fixture fabricated events: %+v, %v", initialMetrics, err)
	}
	for i, userID := range DemoProfileIDs() {
		d, err := engine.Decide(t.Context(), decision.Request{RequestID: userID, UserID: userID, SlotID: DemoSlotID, Now: now})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			if !d.Matched || d.CampaignID != DemoCampaignID || d.CreativeID != DemoCreativeID {
				t.Fatalf("matching example did not deliver: %+v", d)
			}
			for _, kind := range []event.Type{event.Impression, event.Click, event.Conversion} {
				value := int64(0)
				if kind == event.Conversion {
					value = 500
				}
				created, err := events.Record(t.Context(), event.Event{EventID: "demo-" + string(kind), RequestID: d.RequestID, CampaignID: d.CampaignID, CreativeID: d.CreativeID, Type: kind, ValueFen: value, OccurredAt: now})
				if err != nil || !created {
					t.Fatalf("record %s: created=%v err=%v", kind, created, err)
				}
			}
		} else if d.Matched || d.Reason != decision.ReasonTargetingMiss {
			t.Fatalf("negative example delivered: %+v", d)
		}
	}
	metrics, err := events.Metrics(t.Context(), DemoCampaignID)
	if err != nil || metrics.Impressions != 1 || metrics.Clicks != 1 || metrics.Conversions != 1 || metrics.ValueFen != 500 {
		t.Fatalf("event chain metrics=%+v err=%v", metrics, err)
	}
	items, err := provider.ActiveCandidates(t.Context(), DemoSlotID, now)
	if err != nil || len(items) != 1 {
		t.Fatalf("candidates=%+v err=%v", items, err)
	}
	for i, code := range []string{"", "excluded_condition", "missing_tag"} {
		profile, err := runtime.FindProfile(t.Context(), DemoProfileIDs()[i])
		if err != nil {
			t.Fatal(err)
		}
		failures := (decision.Evaluator{}).Explain(profile, items[0].Targeting)
		if code == "" && len(failures) != 0 || code != "" && (len(failures) != 1 || failures[0].Code != code) {
			t.Fatalf("user %s failures=%+v", profile.UserID, failures)
		}
	}
}

func TestMemoryDemoPreservesEditsAndDeletionsOnRepeat(t *testing.T) {
	initializer, campaigns, profiles := memoryDemo(t)
	now := time.Now().UTC()
	if _, err := initializer.Run(t.Context(), now); err != nil {
		t.Fatal(err)
	}
	service := campaignapp.NewService(campaigns, nil)
	if _, err := service.Pause(t.Context(), DemoCampaignID); err != nil {
		t.Fatal(err)
	}
	changed := decision.NewProfile(DemoProfileIDs()[0], []string{"user-edited"}, map[string]string{"score": "1"})
	if err := profiles.PutProfile(t.Context(), changed); err != nil {
		t.Fatal(err)
	}
	if result, err := initializer.Run(t.Context(), now.Add(time.Hour)); err != nil || result != (DemoResult{}) {
		t.Fatalf("repeat=%+v err=%v", result, err)
	}
	c, err := campaigns.FindByID(t.Context(), DemoCampaignID)
	if err != nil || c.Status() != campaign.StatusPaused {
		t.Fatalf("campaign edit lost: %v %v", c, err)
	}
	p, err := profiles.FindProfile(t.Context(), changed.UserID)
	if err != nil || p.Fields["score"] != "1" {
		t.Fatalf("profile edit lost: %+v %v", p, err)
	}
	creativeService := campaignapp.NewCreativeService(campaigns, campaigns)
	if _, err := creativeService.Disable(t.Context(), DemoCampaignID, DemoCreativeID); err != nil {
		t.Fatal(err)
	}
	if err := creativeService.Delete(t.Context(), DemoCampaignID, DemoCreativeID); err != nil {
		t.Fatal(err)
	}
	if err := service.Delete(t.Context(), DemoCampaignID); err != nil {
		t.Fatal(err)
	}
	for _, id := range DemoProfileIDs() {
		if err := profiles.DeleteProfile(t.Context(), id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := initializer.Run(t.Context(), now.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := campaigns.FindByID(t.Context(), DemoCampaignID); !errors.Is(err, campaign.ErrCampaignNotFound) {
		t.Fatalf("campaign restored: %v", err)
	}
	if _, err := campaigns.FindCreativeByID(t.Context(), DemoCreativeID); !errors.Is(err, campaign.ErrCreativeNotFound) {
		t.Fatalf("creative restored: %v", err)
	}
	for _, id := range DemoProfileIDs() {
		if _, err := profiles.FindProfile(t.Context(), id); !errors.Is(err, decision.ErrProfileNotFound) {
			t.Fatalf("profile %s restored: %v", id, err)
		}
	}
	// A process restart creates entirely new stores, so it receives a new fixture.
	fresh, _, _ := memoryDemo(t)
	if result, err := fresh.Run(t.Context(), now); err != nil || !result.CampaignsInstalled || !result.ProfilesInstalled {
		t.Fatalf("fresh=%+v err=%v", result, err)
	}
}

func TestMemoryDemoRejectsExistingReservedProfileWithoutOverwriting(t *testing.T) {
	initializer, _, profiles := memoryDemo(t)
	id := DemoProfileIDs()[1]
	if err := profiles.PutProfile(t.Context(), decision.NewProfile(id, []string{"mine"}, map[string]string{"device": "web"})); err != nil {
		t.Fatal(err)
	}
	if _, err := initializer.Run(t.Context(), time.Now()); err == nil {
		t.Fatal("expected ID conflict")
	}
	p, err := profiles.FindProfile(t.Context(), id)
	if err != nil || p.Fields["device"] != "web" {
		t.Fatalf("overwritten profile=%+v err=%v", p, err)
	}
	if _, err := profiles.FindProfile(t.Context(), DemoProfileIDs()[0]); !errors.Is(err, decision.ErrProfileNotFound) {
		t.Fatalf("wrote partial profile group: %v", err)
	}
}

func TestMemoryDemoConcurrentCallsInstallOnce(t *testing.T) {
	initializer, _, _ := memoryDemo(t)
	results := make(chan DemoResult, 8)
	errors := make(chan error, 8)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() { result, err := initializer.Run(t.Context(), time.Now()); results <- result; errors <- err })
	}
	wg.Wait()
	close(results)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	var campaignInstalls, profileInstalls int
	for result := range results {
		if result.CampaignsInstalled {
			campaignInstalls++
		}
		if result.ProfilesInstalled {
			profileInstalls++
		}
	}
	if campaignInstalls != 1 || profileInstalls != 1 {
		t.Fatalf("installs=%d/%d", campaignInstalls, profileInstalls)
	}
}
