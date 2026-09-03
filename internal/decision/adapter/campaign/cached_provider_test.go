package campaign

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zhanghaiyang/adflow/internal/decision/domain"
)

type candidateSourceFunc func(context.Context, string, time.Time) ([]domain.Candidate, error)

func (f candidateSourceFunc) ActiveCandidates(ctx context.Context, slotID string, now time.Time) ([]domain.Candidate, error) {
	return f(ctx, slotID, now)
}

func cachedCandidate() domain.Candidate {
	return domain.Candidate{
		CampaignID: "campaign-1", CreativeIDs: []string{"creative-1"},
		Targeting: domain.TargetingRule{All: []domain.Condition{{Tag: "anime"}}},
	}
}

func TestCachedProviderReturnsDefensiveSnapshotCopy(t *testing.T) {
	var calls atomic.Int64
	provider, err := NewCachedProvider(candidateSourceFunc(func(context.Context, string, time.Time) ([]domain.Candidate, error) {
		calls.Add(1)
		return []domain.Candidate{cachedCandidate()}, nil
	}), time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	first, err := provider.ActiveCandidates(context.Background(), "slot-1", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	first[0].CreativeIDs[0] = "mutated"
	first[0].Targeting.All[0].Tag = "mutated"
	second, err := provider.ActiveCandidates(context.Background(), "slot-1", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || second[0].CreativeIDs[0] != "creative-1" || second[0].Targeting.All[0].Tag != "anime" {
		t.Fatalf("calls=%d candidates=%+v", calls.Load(), second)
	}
}

func TestCachedProviderReloadsAfterTTL(t *testing.T) {
	var calls atomic.Int64
	provider, err := NewCachedProvider(candidateSourceFunc(func(context.Context, string, time.Time) ([]domain.Candidate, error) {
		calls.Add(1)
		return []domain.Candidate{cachedCandidate()}, nil
	}), time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	provider.now = func() time.Time { return clock }
	if _, err := provider.ActiveCandidates(context.Background(), "slot-1", clock); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(time.Second)
	if _, err := provider.ActiveCandidates(context.Background(), "slot-1", clock); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("source calls=%d", calls.Load())
	}
}

func TestCachedProviderCoalescesConcurrentMisses(t *testing.T) {
	var calls atomic.Int64
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	provider, err := NewCachedProvider(candidateSourceFunc(func(context.Context, string, time.Time) ([]domain.Candidate, error) {
		calls.Add(1)
		once.Do(func() { close(started) })
		<-release
		return []domain.Candidate{cachedCandidate()}, nil
	}), time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	const workers = 32
	start := make(chan struct{})
	errors := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, callErr := provider.ActiveCandidates(context.Background(), "slot-1", time.Now())
			errors <- callErr
		}()
	}
	close(start)
	<-started
	close(release)
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("source calls=%d", calls.Load())
	}
}

func TestCachedProviderBoundsUniqueSlotSnapshots(t *testing.T) {
	provider, err := NewCachedProvider(candidateSourceFunc(func(context.Context, string, time.Time) ([]domain.Candidate, error) {
		return []domain.Candidate{}, nil
	}), time.Minute, nil)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < maxCandidateSnapshots+10; index++ {
		if _, err := provider.ActiveCandidates(context.Background(), "slot-"+strconv.Itoa(index), time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if len(provider.entries) != maxCandidateSnapshots {
		t.Fatalf("snapshot entries=%d", len(provider.entries))
	}
}
