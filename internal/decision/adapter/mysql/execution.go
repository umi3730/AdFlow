package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/zhanghaiyang/adflow/internal/decision/domain"
	"time"
)

func (s *Store) AcquireDecision(ctx context.Context, request domain.Request, owner string, ttl time.Duration) error {
	if owner == "" || ttl <= 0 {
		return domain.ErrInvalidRequest
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	fingerprint := domain.RequestFingerprint(request)
	if _, err := tx.ExecContext(ctx, `INSERT INTO decision_requests (request_id, request_fingerprint) VALUES (?, ?) ON DUPLICATE KEY UPDATE request_id = request_id`, request.RequestID, fingerprint); err != nil {
		return err
	}
	var storedFingerprint string
	var storedOwner sql.NullString
	var active bool
	if err := tx.QueryRowContext(ctx, `SELECT request_fingerprint, owner_id, COALESCE(lease_until > UTC_TIMESTAMP(3), 0) FROM decision_requests WHERE request_id = ? FOR UPDATE`, request.RequestID).Scan(&storedFingerprint, &storedOwner, &active); err != nil {
		return err
	}
	if storedFingerprint != fingerprint {
		return domain.ErrRequestConflict
	}
	if storedOwner.Valid && storedOwner.String != "" && active {
		return domain.ErrDecisionInProgress
	}
	if _, err := tx.ExecContext(ctx, `UPDATE decision_requests SET owner_id = ?, lease_until = TIMESTAMPADD(MICROSECOND, ?, UTC_TIMESTAMP(3)) WHERE request_id = ?`, owner, ttl.Microseconds(), request.RequestID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) CommitDecision(ctx context.Context, result domain.Result, owner string) error {
	notCommitted := func(err error) error { return fmt.Errorf("%w: %w", domain.ErrDecisionNotCommitted, err) }
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return notCommitted(err)
	}
	defer tx.Rollback()
	var fingerprint string
	var currentOwner sql.NullString
	var active bool
	if err := tx.QueryRowContext(ctx, `SELECT request_fingerprint, owner_id, COALESCE(lease_until > UTC_TIMESTAMP(3), 0) FROM decision_requests WHERE request_id = ? FOR UPDATE`, result.RequestID).Scan(&fingerprint, &currentOwner, &active); err != nil {
		if err == sql.ErrNoRows {
			return notCommitted(domain.ErrDecisionExecutionLost)
		}
		return notCommitted(err)
	}
	if !currentOwner.Valid || currentOwner.String != owner || !active || fingerprint != result.RequestFingerprint {
		return notCommitted(domain.ErrDecisionExecutionLost)
	}
	if err := saveDecision(ctx, tx, result); err != nil {
		return notCommitted(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE decision_requests SET owner_id = NULL, lease_until = NULL WHERE request_id = ? AND owner_id = ?`, result.RequestID, owner); err != nil {
		return notCommitted(err)
	}
	// A COMMIT error is ambiguous: the caller must retain reservations until a
	// read-back confirms ownership, or let their bounded TTL expire.
	return tx.Commit()
}

func (s *Store) ReleaseDecision(ctx context.Context, requestID, owner string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE decision_requests SET owner_id = NULL, lease_until = NULL WHERE request_id = ? AND owner_id = ?`, requestID, owner)
	return err
}
