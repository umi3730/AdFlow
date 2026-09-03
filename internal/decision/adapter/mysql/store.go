package mysql

import (
	"context"
	"database/sql"
	"strings"
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
	result, err := scanDecision(s.db.QueryRowContext(ctx, `
		SELECT request_id, user_id, slot_id, matched, campaign_id, creative_id,
		       reservation_token, expires_at, reason
		FROM decisions WHERE request_id = ?`, requestID))
	if err == sql.ErrNoRows {
		return domain.Result{}, false, nil
	}
	return result, err == nil, err
}

func (s *Store) FindDecisions(ctx context.Context, requestIDs []string) (map[string]domain.Result, error) {
	result := make(map[string]domain.Result, len(requestIDs))
	if len(requestIDs) == 0 {
		return result, nil
	}
	unique := make([]string, 0, len(requestIDs))
	seen := make(map[string]struct{}, len(requestIDs))
	for _, requestID := range requestIDs {
		if _, exists := seen[requestID]; !exists {
			seen[requestID] = struct{}{}
			unique = append(unique, requestID)
		}
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(unique)), ",")
	args := make([]any, len(unique))
	for index, requestID := range unique {
		args[index] = requestID
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT request_id, user_id, slot_id, matched, campaign_id, creative_id,
		       reservation_token, expires_at, reason
		FROM decisions WHERE request_id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		decision, err := scanDecision(rows)
		if err != nil {
			return nil, err
		}
		result[decision.RequestID] = decision
	}
	return result, rows.Err()
}

func scanDecision(scanner interface{ Scan(...any) error }) (domain.Result, error) {
	var (
		result                        domain.Result
		campaignID, creativeID, token sql.NullString
		expiresAt                     sql.NullTime
		reason                        string
	)
	err := scanner.Scan(&result.RequestID, &result.UserID, &result.SlotID, &result.Matched, &campaignID, &creativeID, &token, &expiresAt, &reason)
	if err != nil {
		return domain.Result{}, err
	}
	result.CampaignID = campaignID.String
	result.CreativeID = creativeID.String
	result.ReservationToken = token.String
	if expiresAt.Valid {
		result.ExpiresAt = expiresAt.Time.UTC()
	}
	result.Reason = domain.Reason(reason)
	return result, nil
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
