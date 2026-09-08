package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/umi3730/adflow/internal/audit/domain"
	identity "github.com/umi3730/adflow/internal/identity/domain"
)

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) Append(ctx context.Context, entry domain.Entry) error {
	metadata, err := json.Marshal(entry.Metadata)
	if err != nil {
		return fmt.Errorf("marshal audit metadata: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO audit_logs (
			id, actor_id, actor_name, actor_role, action, resource_type,
			resource_id, request_id, outcome, metadata, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.ActorID, entry.ActorName, entry.ActorRole, entry.Action, entry.ResourceType,
		nullable(entry.ResourceID), entry.RequestID, entry.Outcome, json.RawMessage(metadata), entry.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("append audit log: %w", err)
	}
	return nil
}

func (s *Store) List(ctx context.Context, filter domain.Filter) ([]domain.Entry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, actor_id, actor_name, actor_role, action, resource_type,
		       COALESCE(resource_id, ''), request_id, outcome, metadata, created_at
		FROM audit_logs
		ORDER BY created_at DESC, id DESC
		LIMIT ? OFFSET ?`, filter.Limit, filter.Offset)
	if err != nil {
		return nil, fmt.Errorf("list audit logs: %w", err)
	}
	defer rows.Close()
	entries := make([]domain.Entry, 0, filter.Limit)
	for rows.Next() {
		var entry domain.Entry
		var role string
		var outcome string
		var metadata []byte
		if err := rows.Scan(&entry.ID, &entry.ActorID, &entry.ActorName, &role, &entry.Action, &entry.ResourceType,
			&entry.ResourceID, &entry.RequestID, &outcome, &metadata, &entry.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan audit log: %w", err)
		}
		entry.ActorRole = identity.Role(role)
		entry.Outcome = domain.Outcome(outcome)
		if len(metadata) > 0 {
			if err := json.Unmarshal(metadata, &entry.Metadata); err != nil {
				return nil, fmt.Errorf("decode audit metadata: %w", err)
			}
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit logs: %w", err)
	}
	return entries, nil
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}
