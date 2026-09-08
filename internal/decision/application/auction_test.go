package application

import (
	"fmt"
	"github.com/zhanghaiyang/adflow/internal/decision/adapter/memory"
	"github.com/zhanghaiyang/adflow/internal/decision/domain"
	eventmemory "github.com/zhanghaiyang/adflow/internal/event/adapter/memory"
	eventapp "github.com/zhanghaiyang/adflow/internal/event/application"
	eventdomain "github.com/zhanghaiyang/adflow/internal/event/domain"
	"sync"
	"testing"
	"time"
)

func bidCandidate(id, advertiser string, bid int64) domain.Candidate {
	return domain.Candidate{CampaignID: id, AdvertiserID: advertiser, AdvertiserName: advertiser, Version: 2, BidFen: bid, CreativeIDs: []string{"creative-" + id}, Targeting: domain.TargetingRule{All: []domain.Condition{{Tag: "auction"}}}, DailyBudgetFen: 1000, ImpressionCostFen: 1, FrequencyLimit: 100}
}

func TestAuctionHighestBidWinsAndChargesItsOwnPrice(t *testing.T) {
	runtime := memory.NewRuntime()
	_ = runtime.PutProfile(t.Context(), domain.NewProfile("user", []string{"auction"}, nil))
	candidates := []domain.Candidate{bidCandidate("a", "studio-a", 2), bidCandidate("z", "studio-b", 5), bidCandidate("c", "studio-c", 3)}
	engine := NewService(candidateProvider{candidates}, runtime, runtime, runtime, runtime)
	request := domain.Request{RequestID: "bid-1", UserID: "user", SlotID: "slot", Now: time.Now().UTC()}
	result, err := engine.Decide(t.Context(), request)
	if err != nil || !result.Matched || result.CampaignID != "z" || result.Pricing.PriceFen != 5 || result.Pricing.Mode != "first_price" || result.Pricing.Advertisers != 3 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	// Price and identity are the original snapshot, even if later configuration changes.
	candidates[1].BidFen = 9
	again, err := engine.Decide(t.Context(), request)
	if err != nil || again != result {
		t.Fatalf("retry changed settlement: %+v %v", again, err)
	}
}

func TestAuctionBudgetFailureFallsBackAndReleasesFrequency(t *testing.T) {
	runtime := memory.NewRuntime()
	_ = runtime.PutProfile(t.Context(), domain.NewProfile("user", []string{"auction"}, nil))
	high := bidCandidate("high", "studio-a", 5)
	high.DailyBudgetFen = 5
	engine := NewService(candidateProvider{[]domain.Candidate{high, bidCandidate("low", "studio-b", 3)}}, runtime, runtime, runtime, runtime)
	now := time.Now().UTC()
	first, err := engine.Decide(t.Context(), domain.Request{RequestID: "first", UserID: "user", SlotID: "slot", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	events := eventapp.NewService(eventmemory.NewStore(), runtime, runtime)
	impression := eventdomain.Event{EventID: "exposure", RequestID: first.RequestID, CampaignID: first.CampaignID, CreativeID: first.CreativeID, Type: eventdomain.Impression, OccurredAt: now}
	if _, err := events.Record(t.Context(), impression); err != nil {
		t.Fatal(err)
	}
	if created, err := events.Record(t.Context(), impression); err != nil || created {
		t.Fatalf("duplicate created=%v err=%v", created, err)
	}
	next, err := engine.Decide(t.Context(), domain.Request{RequestID: "second", UserID: "user", SlotID: "slot", Now: now})
	if err != nil || next.CampaignID != "low" || next.Pricing.PriceFen != 3 || next.Pricing.BudgetRejected != 1 || next.Pricing.Rank != 2 {
		t.Fatalf("fallback=%+v err=%v", next, err)
	}
	_, allowed, err := runtime.ReserveFrequency(t.Context(), "user", "high", "frequency-probe", 2, now, time.Second)
	if err != nil || !allowed {
		t.Fatal("losing budget attempt leaked frequency")
	}
}

func TestAuctionFrequencyFailureFallsBack(t *testing.T) {
	runtime := memory.NewRuntime()
	_ = runtime.PutProfile(t.Context(), domain.NewProfile("user", []string{"auction"}, nil))
	high := bidCandidate("high", "studio-a", 5)
	high.FrequencyLimit = 1
	engine := NewService(candidateProvider{[]domain.Candidate{high, bidCandidate("low", "studio-b", 3)}}, runtime, runtime, runtime, runtime)
	now := time.Now().UTC()
	first, err := engine.Decide(t.Context(), domain.Request{RequestID: "first", UserID: "user", SlotID: "slot", Now: now})
	if err != nil || first.CampaignID != "high" {
		t.Fatal(err)
	}
	next, err := engine.Decide(t.Context(), domain.Request{RequestID: "next", UserID: "user", SlotID: "slot", Now: now})
	if err != nil || next.CampaignID != "low" || next.Pricing.FrequencyRejected != 1 {
		t.Fatalf("result=%+v err=%v", next, err)
	}
}

func TestAuctionUsesOneStaticRepresentativePerAdvertiserAndStableTies(t *testing.T) {
	profile := domain.NewProfile("user", []string{"auction"}, nil)
	candidates := []domain.Candidate{bidCandidate("a1", "a", 5), bidCandidate("a2", "a", 4), bidCandidate("b1", "b", 5)}
	winners := map[string]int{}
	for i := 0; i < 200; i++ {
		requestID := fmt.Sprint(i)
		ranked, count := rankCandidates(candidates, profile, time.Now(), requestID)
		again, _ := rankCandidates(candidates, profile, time.Now(), requestID)
		if count != 2 || len(ranked) != 2 || ranked[0].CampaignID != again[0].CampaignID {
			t.Fatalf("bad ranking=%+v", ranked)
		}
		for _, candidate := range ranked {
			if candidate.CampaignID == "a2" {
				t.Fatal("self-competition")
			}
		}
		winners[ranked[0].AdvertiserID]++
	}
	if winners["a"] < 60 || winners["b"] < 60 {
		t.Fatalf("tie distribution=%v", winners)
	}
	if candidates[0].CampaignID != "a1" {
		t.Fatal("input was mutated")
	}
}

func TestAuctionFiltersStalePeriodsAndFallsBackToLegacy(t *testing.T) {
	profile := domain.NewProfile("user", []string{"auction"}, nil)
	now := time.Now().UTC()
	expired := bidCandidate("expired", "a", 100)
	expired.EndAt = now
	future := bidCandidate("future", "b", 100)
	future.StartAt = now.Add(time.Second)
	mismatch := bidCandidate("mismatch", "c", 100)
	mismatch.Targeting.All[0].Tag = "other"
	empty := bidCandidate("empty", "d", 100)
	empty.CreativeIDs = nil
	legacy := bidCandidate("legacy", "", 0)
	ranked, count := rankCandidates([]domain.Candidate{expired, future, mismatch, empty, legacy}, profile, now, "request")
	if len(ranked) != 1 || ranked[0].CampaignID != "legacy" || count != 0 {
		t.Fatalf("ranked=%+v count=%d", ranked, count)
	}
}

func TestConcurrentAuctionsCannotReserveBeyondTheWinningBudget(t *testing.T) {
	runtime := memory.NewRuntime()
	_ = runtime.PutProfile(t.Context(), domain.NewProfile("user", []string{"auction"}, nil))
	candidate := bidCandidate("winner", "studio-a", 5)
	candidate.DailyBudgetFen = 50
	engine := NewService(candidateProvider{[]domain.Candidate{candidate}}, runtime, runtime, runtime, runtime)
	results := make(chan domain.Result, 100)
	failures := make(chan error, 100)
	var workers sync.WaitGroup
	for i := 0; i < 100; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			result, err := engine.Decide(t.Context(), domain.Request{RequestID: fmt.Sprintf("parallel-%d", i), UserID: "user", SlotID: "slot"})
			if err != nil {
				failures <- err
			}
			results <- result
		}(i)
	}
	workers.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	wins, total := 0, int64(0)
	for result := range results {
		if result.Matched {
			wins++
			total += result.Pricing.PriceFen
		}
	}
	if wins != 10 || total != 50 {
		t.Fatalf("wins=%d reserved=%d", wins, total)
	}
}
