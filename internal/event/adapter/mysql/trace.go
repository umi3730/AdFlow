package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	decisiondomain "github.com/zhanghaiyang/adflow/internal/decision/domain"
	"github.com/zhanghaiyang/adflow/internal/event/domain"
)

func (o *Outbox) ReadRequestState(ctx context.Context, id string) (domain.RequestState, error) {
	state := domain.RequestState{Events: []domain.TraceEvent{}}
	tx, err := o.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return state, err
	}
	defer tx.Rollback()
	if err := tx.QueryRowContext(ctx, "SELECT UTC_TIMESTAMP(3)").Scan(&state.ObservedAt); err != nil {
		return state, err
	}
	var owner sql.NullString
	var lease sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT owner_id, lease_until FROM decision_requests WHERE request_id = ? AND BINARY request_id = BINARY ?`, id, id).Scan(&owner, &lease)
	if err != nil && err != sql.ErrNoRows {
		return state, err
	}
	if err == nil {
		state.Known = true
		state.Execution = &domain.TraceExecution{Status: "RELEASED"}
		if owner.Valid && owner.String != "" {
			state.Execution.Status = "LEASE_EXPIRED"
			if lease.Valid {
				value := lease.Time.UTC()
				state.Execution.LeaseUntil = &value
				if value.After(state.ObservedAt) {
					state.Execution.Status = "RUNNING"
				}
			}
		}
	}
	var settlement domain.TraceSettlement
	var snapshot []byte
	var settledAt, nextAttempt, lockedUntil sql.NullTime
	var lastError sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT decision_snapshot, status, attempts, accepted_at, settled_at, next_attempt_at, locked_until, last_error
 FROM event_settlements WHERE request_id = ? AND BINARY request_id = BINARY ?`, id, id).Scan(&snapshot, &settlement.Status, &settlement.FailedAttempts, &settlement.AcceptedAt, &settledAt, &nextAttempt, &lockedUntil, &lastError)
	if err != nil && err != sql.ErrNoRows {
		return state, err
	}
	if err == nil {
		var decision decisiondomain.Result
		if err := json.Unmarshal(snapshot, &decision); err != nil {
			return state, err
		}
		state.SavedDecision = &decision
		state.Known = true
		state.Settlement = &settlement
		settlement.LastError = lastError.String
		if settledAt.Valid {
			value := settledAt.Time.UTC()
			settlement.SettledAt = &value
		}
		if lockedUntil.Valid {
			value := lockedUntil.Time.UTC()
			settlement.LockedUntil = &value
		}
		if nextAttempt.Valid && settlement.Status == "PENDING" {
			value := nextAttempt.Time.UTC()
			settlement.NextAttemptAt = &value
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT r.event_id, r.event_type, r.value_fen, r.occurred_at, r.created_at,
 o.status, o.attempts, o.last_error, o.next_attempt_at, o.published_at, p.processed_at
 FROM event_outbox o JOIN event_receipts r ON r.event_id = o.event_id
 LEFT JOIN processed_events p ON p.event_id = o.event_id
   AND BINARY p.event_id = BINARY r.event_id AND BINARY p.request_id = BINARY r.request_id
   AND BINARY p.campaign_id = BINARY r.campaign_id AND BINARY p.creative_id = BINARY r.creative_id
   AND p.event_type = r.event_type AND p.value_fen = r.value_fen AND p.occurred_at = r.occurred_at
 WHERE o.aggregate_key = ? AND BINARY o.aggregate_key = BINARY ?
 ORDER BY r.created_at, CASE r.event_type WHEN 'impression' THEN 0 WHEN 'click' THEN 1 ELSE 2 END, r.event_id LIMIT ?`, id, id, domain.TraceEventLimit+1)
	if err != nil {
		return state, err
	}
	defer rows.Close()
	for rows.Next() {
		var event domain.TraceEvent
		var last sql.NullString
		var next, published, processed sql.NullTime
		if err := rows.Scan(&event.EventID, &event.Type, &event.ValueFen, &event.OccurredAt, &event.AcceptedAt, &event.Status, &event.FailedAttempts, &last, &next, &published, &processed); err != nil {
			return state, err
		}
		state.Known = true
		if len(state.Events) == domain.TraceEventLimit {
			state.Truncated = true
			continue
		}
		event.LastError = last.String
		if next.Valid && (event.Status == "PENDING" || event.Status == "SETTLING") {
			value := next.Time.UTC()
			event.NextAttemptAt = &value
		}
		if published.Valid {
			value := published.Time.UTC()
			event.PublishedAt = &value
		}
		if processed.Valid {
			value := processed.Time.UTC()
			event.ProcessedAt = &value
		}
		state.Events = append(state.Events, event)
	}
	if err := rows.Err(); err != nil {
		return state, err
	}
	if err := rows.Close(); err != nil {
		return state, err
	}
	return state, tx.Commit()
}
