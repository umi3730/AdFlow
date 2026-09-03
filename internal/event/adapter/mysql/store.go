package mysql

import (
	"context"
	"database/sql"
	"errors"

	"github.com/zhanghaiyang/adflow/internal/event/domain"
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) Record(ctx context.Context, event domain.Event) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		INSERT IGNORE INTO processed_events
			(event_id, request_id, campaign_id, creative_id, event_type, value_fen, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		event.EventID, event.RequestID, event.CampaignID, event.CreativeID, string(event.Type), event.ValueFen, event.OccurredAt,
	)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if rows == 0 {
		if err := tx.Commit(); err != nil {
			return false, err
		}
		return false, nil
	}
	var impressions, clicks, conversions uint64
	var valueFen int64
	switch event.Type {
	case domain.Impression:
		impressions = 1
	case domain.Click:
		clicks = 1
	case domain.Conversion:
		conversions = 1
		valueFen = event.ValueFen
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO campaign_metrics (campaign_id, impressions, clicks, conversions, value_fen)
		VALUES (?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			impressions = impressions + VALUES(impressions),
			clicks = clicks + VALUES(clicks),
			conversions = conversions + VALUES(conversions),
			value_fen = value_fen + VALUES(value_fen)`,
		event.CampaignID, impressions, clicks, conversions, valueFen,
	)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) HasImpression(ctx context.Context, requestID string) (bool, error) {
	var found int
	err := s.db.QueryRowContext(ctx, `
		SELECT 1 FROM processed_events
		WHERE request_id = ? AND event_type = 'impression'
		LIMIT 1`, requestID).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) Metrics(ctx context.Context, campaignID string) (domain.Metrics, error) {
	metrics := domain.Metrics{CampaignID: campaignID}
	err := s.db.QueryRowContext(ctx, `
		SELECT impressions, clicks, conversions, value_fen
		FROM campaign_metrics WHERE campaign_id = ?`, campaignID).
		Scan(&metrics.Impressions, &metrics.Clicks, &metrics.Conversions, &metrics.ValueFen)
	if errors.Is(err, sql.ErrNoRows) {
		return metrics, nil
	}
	return metrics, err
}
