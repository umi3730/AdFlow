package mysql

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/zhanghaiyang/adflow/internal/decision/domain"
)

type ProfileStore struct {
	db *sql.DB
}

func NewProfileStore(db *sql.DB) *ProfileStore { return &ProfileStore{db: db} }

func (s *ProfileStore) PutProfile(ctx context.Context, profile domain.Profile) error {
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
		profile.UserID, tagsJSON, fieldsJSON,
	)
	return err
}

func (s *ProfileStore) FindProfile(ctx context.Context, userID string) (domain.Profile, error) {
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
