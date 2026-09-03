package mysql

import (
	"context"
	"database/sql"
	"time"

	"github.com/zhanghaiyang/adflow/internal/decision/domain"
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) SaveDecision(ctx context.Context, result domain.Result) error {
	var expiresAt any
	if !result.ExpiresAt.IsZero() {
		expiresAt = result.ExpiresAt
	}
	inserted, err := s.db.ExecContext(ctx, `
		INSERT IGNORE INTO decisions
			(request_id, user_id, slot_id, matched, campaign_id, creative_id, reservation_token, expires_at, reason)
		VALUES (?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?, ?)`,
		result.RequestID, result.UserID, result.SlotID, result.Matched, result.CampaignID,
		result.CreativeID, result.ReservationToken, expiresAt, string(result.Reason),
	)
	if err != nil {
		return err
	}
	rows, err := inserted.RowsAffected()
	if err != nil || rows == 1 {
		return err
	}
	existing, found, err := s.FindDecision(ctx, result.RequestID)
	if err != nil {
		return err
	}
	if !found || !sameDecision(existing, result) {
		return domain.ErrInvalidRequest
	}
	return nil
}

func (s *Store) FindDecision(ctx context.Context, requestID string) (domain.Result, bool, error) {
	var (
		result                        domain.Result
		campaignID, creativeID, token sql.NullString
		expiresAt                     sql.NullTime
		reason                        string
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT request_id, user_id, slot_id, matched, campaign_id, creative_id,
		       reservation_token, expires_at, reason
		FROM decisions WHERE request_id = ?`, requestID).
		Scan(&result.RequestID, &result.UserID, &result.SlotID, &result.Matched, &campaignID, &creativeID, &token, &expiresAt, &reason)
	if err == sql.ErrNoRows {
		return domain.Result{}, false, nil
	}
	if err != nil {
		return domain.Result{}, false, err
	}
	result.CampaignID = campaignID.String
	result.CreativeID = creativeID.String
	result.ReservationToken = token.String
	if expiresAt.Valid {
		result.ExpiresAt = expiresAt.Time.UTC()
	}
	result.Reason = domain.Reason(reason)
	return result, true, nil
}

func sameDecision(left, right domain.Result) bool {
	return left.RequestID == right.RequestID && left.UserID == right.UserID && left.SlotID == right.SlotID &&
		left.Matched == right.Matched && left.CampaignID == right.CampaignID && left.CreativeID == right.CreativeID &&
		left.ReservationToken == right.ReservationToken && equalTime(left.ExpiresAt, right.ExpiresAt) && left.Reason == right.Reason
}

func equalTime(left, right time.Time) bool {
	if left.IsZero() && right.IsZero() {
		return true
	}
	return left.Equal(right)
}
