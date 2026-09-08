package memory

import (
	"context"
	"fmt"
	"github.com/umi3730/adflow/internal/decision/domain"
	"time"
)

type requestClaim struct {
	fingerprint string
	owner       string
	leaseUntil  time.Time
	retainUntil time.Time
}

func (r *Runtime) AcquireDecision(ctx context.Context, request domain.Request, owner string, ttl time.Duration) error {
	if owner == "" || ttl <= 0 {
		return domain.ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	if !now.Before(r.requestSweep) {
		for id, claim := range r.requestClaims {
			if !now.Before(claim.retainUntil) && !now.Before(claim.leaseUntil) {
				delete(r.requestClaims, id)
			}
		}
		for id, result := range r.decisions {
			if !now.Before(result.expiresAt) {
				delete(r.decisions, id)
			}
		}
		r.requestSweep = now.Add(time.Second)
	}
	fingerprint := domain.RequestFingerprint(request)
	claim, exists := r.requestClaims[request.RequestID]
	if exists && now.Before(claim.retainUntil) && claim.fingerprint != fingerprint {
		return domain.ErrRequestConflict
	}
	if exists && claim.owner != "" && now.Before(claim.leaseUntil) {
		return domain.ErrDecisionInProgress
	}
	r.requestClaims[request.RequestID] = requestClaim{fingerprint: fingerprint, owner: owner, leaseUntil: now.Add(ttl), retainUntil: now.Add(max(ttl, 5*time.Minute))}
	return nil
}

func (r *Runtime) CommitDecision(ctx context.Context, result domain.Result, owner string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%w: %w", domain.ErrDecisionNotCommitted, err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	claim, ok := r.requestClaims[result.RequestID]
	if !ok || claim.owner != owner || !now.Before(claim.leaseUntil) || claim.fingerprint != result.RequestFingerprint {
		return fmt.Errorf("%w: %w", domain.ErrDecisionNotCommitted, domain.ErrDecisionExecutionLost)
	}
	if existing, exists := r.decisions[result.RequestID]; exists && now.Before(existing.expiresAt) && existing.result != result {
		return fmt.Errorf("%w: %w", domain.ErrDecisionNotCommitted, domain.ErrRequestConflict)
	}
	r.decisions[result.RequestID] = storedDecision{result: result, expiresAt: now.Add(5 * time.Minute)}
	claim.owner = ""
	claim.leaseUntil = time.Time{}
	claim.retainUntil = now.Add(5 * time.Minute)
	r.requestClaims[result.RequestID] = claim
	return nil
}

func (r *Runtime) ReleaseDecision(_ context.Context, requestID, owner string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	claim, ok := r.requestClaims[requestID]
	if ok && claim.owner == owner {
		claim.owner = ""
		claim.leaseUntil = time.Time{}
		r.requestClaims[requestID] = claim
	}
	return nil
}
