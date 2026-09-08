package memory

import (
	"context"
	decisiondomain "github.com/zhanghaiyang/adflow/internal/decision/domain"
	"github.com/zhanghaiyang/adflow/internal/event/domain"
	"time"
)

func (s *Store) FindAcceptedEvent(_ context.Context, id string) (domain.Event, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if submission, ok := s.submissions[id]; ok {
		return submission.Event, true, nil
	}
	event, ok := s.events[id]
	return event, ok, nil
}
func (s *Store) FindImpressionSubmission(_ context.Context, id string) (domain.ImpressionSubmission, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	submission, ok := s.submissions[id]
	return submission, ok, nil
}
func (s *Store) PrepareImpression(_ context.Context, event domain.Event, decision decisiondomain.Result, acceptedAt time.Time) (domain.ImpressionSubmission, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if prior, ok := s.submissions[event.EventID]; ok {
		if !domain.SameEventIdentity(prior.Event, event) {
			return domain.ImpressionSubmission{}, domain.ErrEventConflict
		}
		return prior, nil
	}
	if prior, ok := s.events[event.EventID]; ok {
		if !domain.SameEventIdentity(prior, event) {
			return domain.ImpressionSubmission{}, domain.ErrEventConflict
		}
		return domain.ImpressionSubmission{ImpressionReceipt: domain.ImpressionReceipt{Event: prior, AcceptedAt: s.recordedAt[event.EventID]}, Processed: true}, nil
	}
	if id, ok := s.impressionIDs[event.RequestID]; ok && id != event.EventID {
		return domain.ImpressionSubmission{}, domain.ErrEventConflict
	}
	submission := domain.ImpressionSubmission{ImpressionReceipt: domain.ImpressionReceipt{Event: event, AcceptedAt: acceptedAt}, Decision: decision}
	s.submissions[event.EventID] = submission
	s.impressionIDs[event.RequestID] = event.EventID
	return submission, nil
}
func (s *Store) CommitImpression(_ context.Context, id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	submission, ok := s.submissions[id]
	if !ok {
		return false, domain.ErrImpressionRequired
	}
	created, err := s.recordLocked(submission.Event)
	if err != nil {
		return false, err
	}
	submission.Processed = true
	submission.LastError = ""
	s.submissions[id] = submission
	s.recordedAt[id] = submission.AcceptedAt
	return created, nil
}
func (s *Store) FindImpressionReceipt(_ context.Context, requestID string) (domain.ImpressionReceipt, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.impressionIDs[requestID]
	if !ok {
		return domain.ImpressionReceipt{}, false, nil
	}
	event, ok := s.events[id]
	if !ok {
		return domain.ImpressionReceipt{}, false, nil
	}
	return domain.ImpressionReceipt{Event: event, AcceptedAt: s.recordedAt[id]}, true, nil
}
