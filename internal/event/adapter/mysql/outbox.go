package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zhanghaiyang/adflow/internal/event/domain"
)

type Outbox struct {
	db       *sql.DB
	workerID string
}

func NewOutbox(db *sql.DB, workerID string) *Outbox {
	return &Outbox{db: db, workerID: workerID}
}

func (o *Outbox) Enqueue(ctx context.Context, event domain.Event) (bool, error) {
	payload, err := json.Marshal(event)
	if err != nil {
		return false, err
	}
	tx, err := o.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		INSERT IGNORE INTO event_receipts
			(event_id, request_id, campaign_id, creative_id, event_type, value_fen, occurred_at, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'ACCEPTED')`,
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
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO event_outbox (event_id, aggregate_key, payload, status, attempts, next_attempt_at)
		VALUES (?, ?, ?, 'PENDING', 0, ?)`,
		event.EventID, event.RequestID, payload, time.Now().UTC(),
	); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (o *Outbox) ClaimBatch(ctx context.Context, limit int, now time.Time, lease time.Duration) ([]domain.OutboxEntry, error) {
	if limit <= 0 {
		return []domain.OutboxEntry{}, nil
	}
	now = now.UTC().Truncate(time.Millisecond)
	lockedUntil := now.Add(lease).Truncate(time.Millisecond)
	tx, err := o.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `
		UPDATE event_outbox
		SET status = 'PROCESSING', locked_by = ?, locked_until = ?
		WHERE status IN ('PENDING', 'PROCESSING')
		  AND next_attempt_at <= ?
		  AND (locked_until IS NULL OR locked_until < ?)
		ORDER BY created_at ASC
		LIMIT ?`, o.workerID, lockedUntil, now, now, limit)
	if err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT payload, attempts
		FROM event_outbox
		WHERE status = 'PROCESSING' AND locked_by = ? AND locked_until = ?
		ORDER BY created_at ASC`, o.workerID, lockedUntil)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]domain.OutboxEntry, 0)
	for rows.Next() {
		var payload []byte
		var attempts int
		if err := rows.Scan(&payload, &attempts); err != nil {
			return nil, err
		}
		var event domain.Event
		if err := json.Unmarshal(payload, &event); err != nil {
			return nil, err
		}
		entries = append(entries, domain.OutboxEntry{Event: event, Attempts: attempts})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return entries, nil
}

func (o *Outbox) MarkPublished(ctx context.Context, eventID string, publishedAt time.Time) error {
	_, err := o.db.ExecContext(ctx, `
		UPDATE event_outbox
		SET status = 'PUBLISHED', published_at = ?, locked_by = NULL, locked_until = NULL, last_error = NULL
		WHERE event_id = ? AND status = 'PROCESSING' AND locked_by = ?`,
		publishedAt, eventID, o.workerID,
	)
	return err
}

func (o *Outbox) MarkPublishedBatch(ctx context.Context, eventIDs []string, publishedAt time.Time) error {
	if len(eventIDs) == 0 {
		return nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(eventIDs)), ",")
	args := make([]any, 0, len(eventIDs)+2)
	args = append(args, publishedAt)
	for _, eventID := range eventIDs {
		args = append(args, eventID)
	}
	args = append(args, o.workerID)
	result, err := o.db.ExecContext(ctx, `
		UPDATE event_outbox
		SET status = 'PUBLISHED', published_at = ?, locked_by = NULL, locked_until = NULL, last_error = NULL
		WHERE event_id IN (`+placeholders+`) AND status = 'PROCESSING' AND locked_by = ?`, args...)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != int64(len(eventIDs)) {
		return fmt.Errorf("mark published batch updated %d of %d Outbox rows", updated, len(eventIDs))
	}
	return nil
}

func (o *Outbox) MarkFailed(ctx context.Context, eventID, message string, nextAttempt time.Time) error {
	if len(message) > 1024 {
		message = message[:1024]
	}
	_, err := o.db.ExecContext(ctx, `
		UPDATE event_outbox
		SET status = 'PENDING', attempts = attempts + 1, next_attempt_at = ?,
		    locked_by = NULL, locked_until = NULL, last_error = ?
		WHERE event_id = ? AND status = 'PROCESSING' AND locked_by = ?`,
		nextAttempt, message, eventID, o.workerID,
	)
	return err
}

func (o *Outbox) MarkDeadLetter(ctx context.Context, eventID, message string, failedAt time.Time) error {
	if len(message) > 1024 {
		message = message[:1024]
	}
	_, err := o.db.ExecContext(ctx, `
		UPDATE event_outbox
		SET status = 'DEAD_LETTERED', attempts = attempts + 1, dead_lettered_at = ?,
		    locked_by = NULL, locked_until = NULL, last_error = ?
		WHERE event_id = ? AND status = 'PROCESSING' AND locked_by = ?`,
		failedAt, message, eventID, o.workerID,
	)
	return err
}

func (o *Outbox) Stats(ctx context.Context) (domain.OutboxStats, error) {
	rows, err := o.db.QueryContext(ctx, `
		SELECT status, COUNT(*) FROM event_outbox GROUP BY status`)
	if err != nil {
		return domain.OutboxStats{}, err
	}
	defer rows.Close()
	var stats domain.OutboxStats
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return domain.OutboxStats{}, err
		}
		switch status {
		case "PENDING":
			stats.Pending = count
		case "PROCESSING":
			stats.Processing = count
		case "PUBLISHED":
			stats.Published = count
		case "DEAD_LETTERED":
			stats.DeadLettered = count
		}
	}
	return stats, rows.Err()
}

func (o *Outbox) ListOutbox(ctx context.Context, filter domain.OutboxFilter) ([]domain.OutboxRecord, error) {
	query := `SELECT payload, status, attempts, next_attempt_at, locked_by, locked_until,
		published_at, dead_lettered_at, last_error, created_at FROM event_outbox`
	args := make([]any, 0, 3)
	if filter.Status != "" {
		query += " WHERE status = ?"
		args = append(args, filter.Status)
	}
	query += " ORDER BY created_at DESC, event_id ASC LIMIT ? OFFSET ?"
	args = append(args, filter.Limit, filter.Offset)
	rows, err := o.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]domain.OutboxRecord, 0)
	for rows.Next() {
		var record domain.OutboxRecord
		var payload []byte
		var lockedBy, lastError sql.NullString
		var lockedUntil, publishedAt, deadLetteredAt sql.NullTime
		if err := rows.Scan(&payload, &record.Status, &record.Attempts, &record.NextAttemptAt, &lockedBy, &lockedUntil, &publishedAt, &deadLetteredAt, &lastError, &record.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(payload, &record.Event); err != nil {
			return nil, err
		}
		record.LockedBy, record.LastError = lockedBy.String, lastError.String
		if lockedUntil.Valid {
			value := lockedUntil.Time.UTC()
			record.LockedUntil = &value
		}
		if publishedAt.Valid {
			value := publishedAt.Time.UTC()
			record.PublishedAt = &value
		}
		if deadLetteredAt.Valid {
			value := deadLetteredAt.Time.UTC()
			record.DeadLetteredAt = &value
		}
		record.NextAttemptAt, record.CreatedAt = record.NextAttemptAt.UTC(), record.CreatedAt.UTC()
		records = append(records, record)
	}
	return records, rows.Err()
}

func (o *Outbox) ReplayDeadLetter(ctx context.Context, eventID string, now time.Time) error {
	result, err := o.db.ExecContext(ctx, `UPDATE event_outbox
		SET status = 'PENDING', attempts = 0, next_attempt_at = ?, locked_by = NULL,
			locked_until = NULL, dead_lettered_at = NULL, last_error = NULL
		WHERE event_id = ? AND status = 'DEAD_LETTERED'`, now.UTC(), eventID)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil || updated == 1 {
		return err
	}
	var status string
	if err := o.db.QueryRowContext(ctx, `SELECT status FROM event_outbox WHERE event_id = ?`, eventID).Scan(&status); err == sql.ErrNoRows {
		return domain.ErrOutboxEntryNotFound
	} else if err != nil {
		return err
	}
	return domain.ErrOutboxNotDeadLetter
}
