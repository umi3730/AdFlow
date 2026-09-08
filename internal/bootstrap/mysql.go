package bootstrap

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"

	decision "github.com/umi3730/adflow/internal/decision/domain"
)

func (s *DemoInitializer) mysqlGroup(ctx context.Context, key string, install func(*sql.Tx) error) (bool, error) {
	tx, err := s.options.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	// The unique-key insert locks this group even when its marker does not yet
	// exist. A competing startup waits for commit/rollback before reading it.
	if _, err := tx.ExecContext(ctx, `INSERT INTO bootstrap_seeds (seed_key) VALUES (?) ON DUPLICATE KEY UPDATE seed_key = VALUES(seed_key)`, key); err != nil {
		return false, err
	}
	var completed sql.NullTime
	if err := tx.QueryRowContext(ctx, `SELECT completed_at FROM bootstrap_seeds WHERE seed_key = ? FOR UPDATE`, key).Scan(&completed); err != nil {
		return false, err
	}
	if completed.Valid {
		return false, tx.Commit()
	}
	if err := install(tx); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE bootstrap_seeds SET completed_at = UTC_TIMESTAMP(3) WHERE seed_key = ?`, key); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func insertCampaign(ctx context.Context, tx *sql.Tx, fixture demoFixture) error {
	c := fixture.campaign
	v := c.ActiveVersion()
	targeting, err := json.Marshal(v.Targeting())
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO campaigns (id, name, status, slot_id, start_at, end_at, active_version, revision) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, c.ID(), string(c.Name()), string(c.Status()), string(c.SlotID()), c.Period().Start(), c.Period().End(), v.Number(), c.Revision()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO campaign_versions (campaign_id, version, targeting_rule, daily_budget, impression_cost, frequency_limit, published_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, c.ID(), v.Number(), json.RawMessage(targeting), v.DailyBudget().Amount(), v.ImpressionCost().Amount(), v.FrequencyLimit(), v.PublishedAt()); err != nil {
		return err
	}
	creative := fixture.creative
	_, err = tx.ExecContext(ctx, `INSERT INTO creatives (id, campaign_id, title, description, image_url, landing_url, status, revision) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, creative.ID(), creative.CampaignID(), creative.Title(), creative.Description(), creative.ImageURL(), creative.LandingURL(), string(creative.Status()), creative.Revision())
	return err
}

func insertProfiles(ctx context.Context, tx *sql.Tx, profiles []decision.Profile) error {
	for _, profile := range profiles {
		tags := make([]string, 0, len(profile.Tags))
		for tag := range profile.Tags {
			tags = append(tags, tag)
		}
		sort.Strings(tags)
		tagsJSON, err := json.Marshal(tags)
		if err != nil {
			return err
		}
		fieldsJSON, err := json.Marshal(profile.Fields)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO user_profiles (user_id, tags, fields) VALUES (?, ?, ?)`, profile.UserID, json.RawMessage(tagsJSON), json.RawMessage(fieldsJSON)); err != nil {
			return err
		}
	}
	return nil
}
