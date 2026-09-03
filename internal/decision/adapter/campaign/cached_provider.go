package campaign

import (
	"context"
	"errors"
	"sync"
	"time"

	decisiondomain "github.com/zhanghaiyang/adflow/internal/decision/domain"
)

const maxCandidateSnapshots = 1024

type CandidateCacheObserver interface {
	ObserveCandidateCache(string, time.Duration)
}

type candidateSnapshot struct {
	candidates []decisiondomain.Candidate
	expiresAt  time.Time
}

type candidateLoad struct {
	done       chan struct{}
	candidates []decisiondomain.Candidate
	err        error
}

type CachedProvider struct {
	source   decisiondomain.CandidateProvider
	ttl      time.Duration
	observer CandidateCacheObserver

	mu       sync.Mutex
	entries  map[string]candidateSnapshot
	inFlight map[string]*candidateLoad
	now      func() time.Time
}

func NewCachedProvider(source decisiondomain.CandidateProvider, ttl time.Duration, observer CandidateCacheObserver) (*CachedProvider, error) {
	if source == nil || ttl <= 0 {
		return nil, errors.New("candidate cache source and positive TTL are required")
	}
	return &CachedProvider{
		source: source, ttl: ttl, observer: observer,
		entries: make(map[string]candidateSnapshot), inFlight: make(map[string]*candidateLoad), now: time.Now,
	}, nil
}

func (p *CachedProvider) ActiveCandidates(ctx context.Context, slotID string, decisionTime time.Time) ([]decisiondomain.Candidate, error) {
	cacheTime := p.now()
	p.mu.Lock()
	if entry, exists := p.entries[slotID]; exists && cacheTime.Before(entry.expiresAt) {
		candidates := cloneCandidates(entry.candidates)
		p.mu.Unlock()
		p.observe("hit", 0)
		return candidates, nil
	}
	if load, exists := p.inFlight[slotID]; exists {
		p.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-load.done:
			p.observe("shared", 0)
			return cloneCandidates(load.candidates), load.err
		}
	}
	load := &candidateLoad{done: make(chan struct{})}
	p.inFlight[slotID] = load
	p.mu.Unlock()

	started := time.Now()
	candidates, err := p.source.ActiveCandidates(ctx, slotID, decisionTime)
	duration := time.Since(started)
	owned := cloneCandidates(candidates)
	p.mu.Lock()
	load.candidates = owned
	load.err = err
	if err == nil {
		if _, exists := p.entries[slotID]; !exists && len(p.entries) >= maxCandidateSnapshots {
			p.evictOldestLocked()
		}
		p.entries[slotID] = candidateSnapshot{candidates: cloneCandidates(owned), expiresAt: p.now().Add(p.ttl)}
	}
	delete(p.inFlight, slotID)
	close(load.done)
	p.mu.Unlock()
	if err != nil {
		p.observe("error", duration)
		return nil, err
	}
	p.observe("miss", duration)
	return cloneCandidates(owned), nil
}

func (p *CachedProvider) evictOldestLocked() {
	var oldestSlot string
	var oldestExpiry time.Time
	for slotID, entry := range p.entries {
		if oldestSlot == "" || entry.expiresAt.Before(oldestExpiry) {
			oldestSlot = slotID
			oldestExpiry = entry.expiresAt
		}
	}
	if oldestSlot != "" {
		delete(p.entries, oldestSlot)
	}
}

func (p *CachedProvider) observe(result string, duration time.Duration) {
	if p.observer != nil {
		p.observer.ObserveCandidateCache(result, duration)
	}
}

func cloneCandidates(input []decisiondomain.Candidate) []decisiondomain.Candidate {
	result := make([]decisiondomain.Candidate, len(input))
	for index, candidate := range input {
		result[index] = candidate
		result[index].CreativeIDs = append([]string(nil), candidate.CreativeIDs...)
		result[index].Targeting.All = append([]decisiondomain.Condition(nil), candidate.Targeting.All...)
		result[index].Targeting.Any = append([]decisiondomain.Condition(nil), candidate.Targeting.Any...)
		result[index].Targeting.None = append([]decisiondomain.Condition(nil), candidate.Targeting.None...)
	}
	return result
}
