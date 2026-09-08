package memory

import (
	"context"
	"github.com/umi3730/adflow/internal/event/domain"
	"time"
)

func (s *Store) ObserveSettlementAttempt(_ context.Context, id string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	submission, ok := s.submissions[id]
	if !ok || submission.Processed {
		return
	}
	if err != nil {
		submission.FailedAttempts++
		message := []rune(err.Error())
		if len(message) > 1024 {
			message = message[:1024]
		}
		submission.LastError = string(message)
	} else {
		submission.LastError = ""
	}
	s.submissions[id] = submission
}
func (s *Store) ReadRequestState(ctx context.Context, id string) (domain.RequestState, error) {
	state := domain.RequestState{ObservedAt: time.Now().UTC(), Events: []domain.TraceEvent{}}
	if err := ctx.Err(); err != nil {
		return state, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if eventID, ok := s.impressionIDs[id]; ok {
		if submission, ok := s.submissions[eventID]; ok {
			saved := submission.Decision
			state.SavedDecision = &saved
			state.Known = true
			status := "UNCONFIRMED"
			if submission.Processed {
				status = "SETTLED"
			}
			state.Settlement = &domain.TraceSettlement{Status: status, AcceptedAt: submission.AcceptedAt, FailedAttempts: submission.FailedAttempts, LastError: submission.LastError}
			if !submission.Processed {
				state.Events = append(state.Events, domain.TraceEvent{EventID: eventID, Type: domain.Impression, OccurredAt: submission.Event.OccurredAt, AcceptedAt: submission.AcceptedAt, Status: "ACCEPTED", LastError: submission.LastError})
			}
		}
	}
	for _, eventID := range s.eventsByRequest[id] {
		state.Known = true
		if len(state.Events) >= domain.TraceEventLimit {
			state.Truncated = true
			break
		}
		event := s.events[eventID]
		processed := s.processedAt[eventID]
		state.Events = append(state.Events, domain.TraceEvent{EventID: eventID, Type: event.Type, ValueFen: event.ValueFen, OccurredAt: event.OccurredAt, AcceptedAt: s.recordedAt[eventID], Status: "PROCESSED", ProcessedAt: &processed})
	}
	return state, nil
}
