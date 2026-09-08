package mysql

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/umi3730/adflow/internal/event/domain"
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) RecordBatch(ctx context.Context, events []domain.Event) ([]bool, error) {
	created := make([]bool, len(events))
	if len(events) == 0 {
		return created, nil
	}
	batchID, err := processingBatchID()
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	values := make([]string, len(events))
	args := make([]any, 0, len(events)*8)
	firstIndex := make(map[string]int, len(events))
	for index, event := range events {
		values[index] = "(?, ?, ?, ?, ?, ?, ?, ?)"
		args = append(args, event.EventID, event.RequestID, event.CampaignID, event.CreativeID, string(event.Type), event.ValueFen, event.OccurredAt, batchID)
		if _, exists := firstIndex[event.EventID]; !exists {
			firstIndex[event.EventID] = index
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT IGNORE INTO processed_events
			(event_id, request_id, campaign_id, creative_id, event_type, value_fen, occurred_at, processing_batch_id)
		VALUES `+strings.Join(values, ","), args...); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT event_id, campaign_id, event_type, value_fen
		FROM processed_events WHERE processing_batch_id = ?`, batchID)
	if err != nil {
		return nil, err
	}
	type totals struct {
		impressions uint64
		clicks      uint64
		conversions uint64
		valueFen    int64
	}
	byCampaign := make(map[string]totals)
	for rows.Next() {
		var eventID, campaignID, eventType string
		var valueFen int64
		if err := rows.Scan(&eventID, &campaignID, &eventType, &valueFen); err != nil {
			rows.Close()
			return nil, err
		}
		if index, exists := firstIndex[eventID]; exists {
			created[index] = true
		}
		total := byCampaign[campaignID]
		switch domain.Type(eventType) {
		case domain.Impression:
			total.impressions++
		case domain.Click:
			total.clicks++
		case domain.Conversion:
			total.conversions++
			total.valueFen += valueFen
		}
		byCampaign[campaignID] = total
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(byCampaign) > 0 {
		metricValues := make([]string, 0, len(byCampaign))
		metricArgs := make([]any, 0, len(byCampaign)*5)
		campaignIDs := make([]string, 0, len(byCampaign))
		for campaignID := range byCampaign {
			campaignIDs = append(campaignIDs, campaignID)
		}
		sort.Strings(campaignIDs)
		for _, campaignID := range campaignIDs {
			total := byCampaign[campaignID]
			metricValues = append(metricValues, "(?, ?, ?, ?, ?)")
			metricArgs = append(metricArgs, campaignID, total.impressions, total.clicks, total.conversions, total.valueFen)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO campaign_metrics (campaign_id, impressions, clicks, conversions, value_fen)
			VALUES `+strings.Join(metricValues, ",")+`
			ON DUPLICATE KEY UPDATE
				impressions = impressions + VALUES(impressions),
				clicks = clicks + VALUES(clicks),
				conversions = conversions + VALUES(conversions),
				value_fen = value_fen + VALUES(value_fen)`, metricArgs...); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return created, nil
}

func (s *Store) HasImpressions(ctx context.Context, requestIDs []string) (map[string]bool, error) {
	result := make(map[string]bool, len(requestIDs))
	if len(requestIDs) == 0 {
		return result, nil
	}
	unique := make([]string, 0, len(requestIDs))
	for _, requestID := range requestIDs {
		if _, exists := result[requestID]; !exists {
			result[requestID] = false
			unique = append(unique, requestID)
		}
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(unique)), ",")
	args := make([]any, len(unique))
	for index, requestID := range unique {
		args[index] = requestID
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT request_id FROM processed_events
		WHERE event_type = 'impression' AND request_id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var requestID string
		if err := rows.Scan(&requestID); err != nil {
			return nil, err
		}
		result[requestID] = true
	}
	return result, rows.Err()
}

func processingBatchID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("create event processing batch ID: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}

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
