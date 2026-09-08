package memory

import (
	"context"
	"sync"
	"time"

	"github.com/zhanghaiyang/adflow/internal/event/domain"
)

type Store struct {
	mu              sync.RWMutex
	events          map[string]domain.Event
	impressions     map[string]struct{}
	metrics         map[string]domain.Metrics
	impressionIDs   map[string]string
	submissions     map[string]domain.ImpressionSubmission
	recordedAt      map[string]time.Time
	processedAt     map[string]time.Time
	eventsByRequest map[string][]string
}

func NewStore() *Store {
	return &Store{events: make(map[string]domain.Event), impressions: make(map[string]struct{}), metrics: make(map[string]domain.Metrics), impressionIDs: make(map[string]string), submissions: make(map[string]domain.ImpressionSubmission), recordedAt: make(map[string]time.Time), processedAt: make(map[string]time.Time), eventsByRequest: make(map[string][]string)}
}

func (s *Store) Record(_ context.Context, event domain.Event) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.recordLocked(event)
}

func (s *Store) recordLocked(event domain.Event) (bool, error) {
	if accepted, exists := s.events[event.EventID]; exists {
		if !domain.SameEventIdentity(accepted, event) {
			return false, domain.ErrEventConflict
		}
		return false, nil
	}
	if pending, exists := s.submissions[event.EventID]; exists && !domain.SameEventIdentity(pending.Event, event) {
		return false, domain.ErrEventConflict
	}
	if id, exists := s.impressionIDs[event.RequestID]; event.Type == domain.Impression && exists && id != event.EventID {
		return false, domain.ErrEventConflict
	}
	s.events[event.EventID] = event
	s.recordedAt[event.EventID] = time.Now().UTC()
	s.processedAt[event.EventID] = s.recordedAt[event.EventID]
	s.eventsByRequest[event.RequestID] = append(s.eventsByRequest[event.RequestID], event.EventID)
	metric := s.metrics[event.CampaignID]
	metric.CampaignID = event.CampaignID
	switch event.Type {
	case domain.Impression:
		metric.Impressions++
		s.impressionIDs[event.RequestID] = event.EventID
		s.impressions[event.RequestID] = struct{}{}
	case domain.Click:
		metric.Clicks++
	case domain.Conversion:
		metric.Conversions++
		metric.ValueFen += event.ValueFen
	}
	s.metrics[event.CampaignID] = metric
	return true, nil
}

func (s *Store) RecordBatch(_ context.Context, events []domain.Event) ([]bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	created := make([]bool, len(events))
	identities := make(map[string]domain.Event, len(events))
	impressions := make(map[string]string, len(events))
	// Validate the whole batch before changing metrics.
	for _, event := range events {
		prior, exists := identities[event.EventID]
		if !exists {
			prior, exists = s.events[event.EventID]
		}
		if !exists {
			if pending, ok := s.submissions[event.EventID]; ok {
				prior, exists = pending.Event, true
			}
		}
		if exists && !domain.SameEventIdentity(prior, event) {
			return nil, domain.ErrEventConflict
		}
		if event.Type == domain.Impression {
			id, exists := impressions[event.RequestID]
			if !exists {
				id, exists = s.impressionIDs[event.RequestID]
			}
			if exists && id != event.EventID {
				return nil, domain.ErrEventConflict
			}
			impressions[event.RequestID] = event.EventID
		}
		identities[event.EventID] = event
	}
	for index, event := range events {
		created[index], _ = s.recordLocked(event)
	}
	return created, nil
}

func (s *Store) HasImpressions(_ context.Context, requestIDs []string) (map[string]bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]bool, len(requestIDs))
	for _, requestID := range requestIDs {
		_, result[requestID] = s.impressions[requestID]
	}
	return result, nil
}

func (s *Store) HasImpression(_ context.Context, requestID string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, exists := s.impressions[requestID]
	return exists, nil
}

func (s *Store) Metrics(_ context.Context, campaignID string) (domain.Metrics, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	metric := s.metrics[campaignID]
	metric.CampaignID = campaignID
	return metric, nil
}
