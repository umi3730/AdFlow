package memory

import (
	"context"
	"sync"

	"github.com/zhanghaiyang/adflow/internal/event/domain"
)

type Store struct {
	mu          sync.RWMutex
	events      map[string]domain.Event
	impressions map[string]struct{}
	metrics     map[string]domain.Metrics
}

func NewStore() *Store {
	return &Store{events: make(map[string]domain.Event), impressions: make(map[string]struct{}), metrics: make(map[string]domain.Metrics)}
}

func (s *Store) Record(_ context.Context, event domain.Event) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.events[event.EventID]; exists {
		return false, nil
	}
	s.events[event.EventID] = event
	metric := s.metrics[event.CampaignID]
	metric.CampaignID = event.CampaignID
	switch event.Type {
	case domain.Impression:
		metric.Impressions++
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
	for index, event := range events {
		if _, exists := s.events[event.EventID]; exists {
			continue
		}
		created[index] = true
		s.events[event.EventID] = event
		metric := s.metrics[event.CampaignID]
		metric.CampaignID = event.CampaignID
		switch event.Type {
		case domain.Impression:
			metric.Impressions++
			s.impressions[event.RequestID] = struct{}{}
		case domain.Click:
			metric.Clicks++
		case domain.Conversion:
			metric.Conversions++
			metric.ValueFen += event.ValueFen
		}
		s.metrics[event.CampaignID] = metric
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
