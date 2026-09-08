package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/umi3730/adflow/internal/decision/domain"
)

func newExecutionID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}

func executionReservationID(owner, campaignID string) string {
	digest := sha256.Sum256([]byte(owner + "\x00" + campaignID))
	return "rsv-" + hex.EncodeToString(digest[:])
}

func (s *Service) commitDecision(ctx context.Context, request domain.Request, result domain.Result, owner, frequencyToken, budgetToken string) (domain.Result, error) {
	result.RequestFingerprint = domain.RequestFingerprint(request)
	err := s.decisions.CommitDecision(ctx, result, owner)
	if err == nil {
		return result, nil
	}
	// A lost COMMIT acknowledgement must not undo a successfully stored decision.
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), reservationCleanupTimeout)
	defer cancel()
	existing, found, readErr := s.decisions.FindDecision(readCtx, request.RequestID)
	if readErr == nil && found {
		if budgetToken != "" && existing.ReservationToken != budgetToken {
			s.releaseReservations(ctx, frequencyToken, budgetToken)
		}
		if !domain.MatchesRequest(existing, request) {
			return domain.Result{}, domain.ErrRequestConflict
		}
		return existing, nil
	}
	// Only a definite rollback allows eager release. Uncertain outcomes retain
	// this attempt's reservation for its bounded TTL, never another owner's token.
	if errors.Is(err, domain.ErrDecisionNotCommitted) && budgetToken != "" {
		s.releaseReservations(ctx, frequencyToken, budgetToken)
	}
	return domain.Result{}, err
}
