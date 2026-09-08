package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/zhanghaiyang/adflow/internal/decision/domain"
)

type ProfileStore struct {
	db *sql.DB
}

func NewProfileStore(db *sql.DB) *ProfileStore { return &ProfileStore{db: db} }

func (s *ProfileStore) DeleteProfile(ctx context.Context, userID string) error {
	userID = strings.TrimSpace(userID)
	_, err := s.db.ExecContext(ctx, "DELETE FROM user_profiles WHERE user_id = ?", userID)
	return err
}

func (s *ProfileStore) PutProfile(ctx context.Context, profile domain.Profile) error {
	profile.UserID = strings.TrimSpace(profile.UserID)
	if profile.UserID == "" {
		return domain.ErrInvalidRequest
	}
	tags := make([]string, 0, len(profile.Tags))
	for tag := range profile.Tags {
		tags = append(tags, tag)
	}
	tagsJSON, err := json.Marshal(tags)
	if err != nil {
		return err
	}
	fieldsJSON, err := json.Marshal(profile.Fields)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO user_profiles (user_id, tags, fields)
		VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE tags = VALUES(tags), fields = VALUES(fields)`,
		profile.UserID, json.RawMessage(tagsJSON), json.RawMessage(fieldsJSON),
	)
	return err
}

func (s *ProfileStore) FindProfile(ctx context.Context, userID string) (domain.Profile, error) {
	userID = strings.TrimSpace(userID)
	var tagsJSON, fieldsJSON []byte
	if err := s.db.QueryRowContext(ctx, `
		SELECT tags, fields FROM user_profiles WHERE user_id = ?`, userID).
		Scan(&tagsJSON, &fieldsJSON); err != nil {
		if err == sql.ErrNoRows {
			return domain.Profile{}, domain.ErrProfileNotFound
		}
		return domain.Profile{}, err
	}
	var tags []string
	var fields map[string]string
	if err := json.Unmarshal(tagsJSON, &tags); err != nil {
		return domain.Profile{}, err
	}
	if err := json.Unmarshal(fieldsJSON, &fields); err != nil {
		return domain.Profile{}, err
	}
	return domain.NewProfile(userID, tags, fields), nil
}

func (s *ProfileStore) ListProfiles(ctx context.Context, filter domain.ProfileFilter) (domain.ProfilePage, error) {
	conditions := make([]string, 0, 3)
	args := make([]any, 0, 5)
	if filter.Query != "" {
		conditions = append(conditions, "LOCATE(?, user_id) > 0")
		args = append(args, filter.Query)
	}
	if filter.Tag != "" {
		conditions = append(conditions, "JSON_CONTAINS(tags, JSON_QUOTE(?))")
		args = append(args, filter.Tag)
	}
	if filter.Device != "" {
		conditions = append(conditions, "JSON_UNQUOTE(JSON_EXTRACT(fields, '$.device')) = ?")
		args = append(args, filter.Device)
	}
	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}
	page := domain.ProfilePage{Items: make([]domain.Profile, 0)}
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM user_profiles"+where, args...).Scan(&page.Total); err != nil {
		return page, err
	}
	limit, offset := filter.Limit, max(filter.Offset, 0)
	if limit < 1 || limit > 100 {
		limit = 20
	}
	args = append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, "SELECT user_id, tags, fields FROM user_profiles"+where+" ORDER BY user_id ASC LIMIT ? OFFSET ?", args...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var userID string
		var tagsJSON, fieldsJSON []byte
		if err := rows.Scan(&userID, &tagsJSON, &fieldsJSON); err != nil {
			return page, err
		}
		var tags []string
		var fields map[string]string
		if err := json.Unmarshal(tagsJSON, &tags); err != nil {
			return page, err
		}
		if err := json.Unmarshal(fieldsJSON, &fields); err != nil {
			return page, err
		}
		page.Items = append(page.Items, domain.NewProfile(userID, tags, fields))
	}
	return page, rows.Err()
}
