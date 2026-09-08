package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/umi3730/adflow/internal/campaign/domain"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Create(ctx context.Context, campaign *domain.Campaign) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO campaigns (id, name, status, slot_id, start_at, end_at, active_version, revision)
		VALUES (?, ?, ?, ?, ?, ?, NULL, ?)`,
		campaign.ID(), string(campaign.Name()), string(campaign.Status()), string(campaign.SlotID()),
		campaign.Period().Start(), campaign.Period().End(), campaign.Revision(),
	)
	return err
}

func (r *Repository) Save(ctx context.Context, campaign *domain.Campaign, expectedRevision uint64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var activeVersion any
	if version := campaign.ActiveVersion(); version != nil {
		activeVersion = version.Number()
		targeting, marshalErr := json.Marshal(version.Targeting())
		if marshalErr != nil {
			return marshalErr
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO campaign_versions
				(campaign_id, version, targeting_rule, daily_budget, impression_cost, frequency_limit, published_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE campaign_id = VALUES(campaign_id)`,
			campaign.ID(), version.Number(), json.RawMessage(targeting), version.DailyBudget().Amount(),
			version.ImpressionCost().Amount(), version.FrequencyLimit(), version.PublishedAt(),
		)
		if err != nil {
			return err
		}
		if terms := version.Auction(); terms != nil {
			payload, marshalErr := json.Marshal(terms)
			if marshalErr != nil {
				return marshalErr
			}
			if _, err := tx.ExecContext(ctx, `UPDATE campaign_versions SET auction_terms = ? WHERE campaign_id = ? AND version = ? AND auction_terms IS NULL`, json.RawMessage(payload), campaign.ID(), version.Number()); err != nil {
				return err
			}
		}
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE campaigns
		SET name = ?, status = ?, slot_id = ?, start_at = ?, end_at = ?, active_version = ?, revision = ?
		WHERE id = ? AND revision = ?`,
		string(campaign.Name()), string(campaign.Status()), string(campaign.SlotID()), campaign.Period().Start(),
		campaign.Period().End(), activeVersion, campaign.Revision(), campaign.ID(), expectedRevision,
	)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return domain.ErrConcurrentMutation
	}
	return tx.Commit()
}

const campaignSelect = `
	SELECT c.id, c.name, c.slot_id, c.start_at, c.end_at, c.status, c.revision,
	       v.version, v.targeting_rule, v.daily_budget, v.impression_cost, v.frequency_limit, v.published_at, v.auction_terms
	FROM campaigns c
	LEFT JOIN campaign_versions v ON v.campaign_id = c.id AND v.version = c.active_version`

func (r *Repository) FindByID(ctx context.Context, id string) (*domain.Campaign, error) {
	campaign, err := scanCampaign(r.db.QueryRowContext(ctx, campaignSelect+" WHERE c.id = ? AND c.status <> 'DELETED'", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrCampaignNotFound
	}
	return campaign, err
}

func (r *Repository) List(ctx context.Context, filter domain.ListFilter) ([]*domain.Campaign, error) {
	query := campaignSelect
	conditions := []string{"c.status <> 'DELETED'"}
	args := make([]any, 0, 4)
	if filter.Status != nil {
		conditions = append(conditions, "c.status = ?")
		args = append(args, string(*filter.Status))
	}
	if filter.SlotID != nil {
		conditions = append(conditions, "c.slot_id = ?")
		args = append(args, string(*filter.SlotID))
	}
	if filter.AfterID != nil {
		conditions = append(conditions, "c.id > ?")
		args = append(args, *filter.AfterID)
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	if filter.AfterID != nil {
		query += " ORDER BY c.id ASC LIMIT ? OFFSET ?"
	} else {
		query += " ORDER BY c.created_at DESC, c.id ASC LIMIT ? OFFSET ?"
	}
	args = append(args, filter.Limit, filter.Offset)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]*domain.Campaign, 0)
	for rows.Next() {
		campaign, scanErr := scanCampaign(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, campaign)
	}
	return result, rows.Err()
}

type rowScanner interface {
	Scan(...any) error
}

func scanCampaign(scanner rowScanner) (*domain.Campaign, error) {
	var (
		id, nameValue, slotValue, statusValue string
		startAt, endAt                        sql.NullTime
		revision                              uint64
		versionNumber                         sql.NullInt64
		targetingJSON                         []byte
		auctionJSON                           []byte
		dailyBudget, impressionCost           sql.NullInt64
		frequencyLimit                        sql.NullInt64
		publishedAt                           sql.NullTime
	)
	if err := scanner.Scan(
		&id, &nameValue, &slotValue, &startAt, &endAt, &statusValue, &revision,
		&versionNumber, &targetingJSON, &dailyBudget, &impressionCost, &frequencyLimit, &publishedAt, &auctionJSON,
	); err != nil {
		return nil, err
	}
	name, err := domain.NewName(nameValue)
	if err != nil {
		return nil, fmt.Errorf("rehydrate campaign name: %w", err)
	}
	slotID, err := domain.NewSlotID(slotValue)
	if err != nil {
		return nil, fmt.Errorf("rehydrate campaign slot: %w", err)
	}
	period, err := domain.NewDeliveryPeriod(startAt.Time, endAt.Time)
	if err != nil {
		return nil, fmt.Errorf("rehydrate campaign period: %w", err)
	}
	var version *domain.Version
	if versionNumber.Valid {
		var raw domain.TargetingRule
		if err := json.Unmarshal(targetingJSON, &raw); err != nil {
			return nil, fmt.Errorf("decode targeting rule: %w", err)
		}
		rule, err := domain.NewTargetingRule(raw.All, raw.Any, raw.None)
		if err != nil {
			return nil, fmt.Errorf("rehydrate targeting rule: %w", err)
		}
		var auction *domain.AuctionTerms
		if len(auctionJSON) > 0 {
			if err := json.Unmarshal(auctionJSON, &auction); err != nil {
				return nil, fmt.Errorf("decode auction: %w", err)
			}
		}
		rehydrated, err := domain.NewAuctionVersion(
			uint32(versionNumber.Int64), rule, dailyBudget.Int64, impressionCost.Int64,
			uint32(frequencyLimit.Int64), publishedAt.Time, auction,
		)
		if err != nil {
			return nil, fmt.Errorf("rehydrate campaign version: %w", err)
		}
		version = &rehydrated
	}
	return domain.Rehydrate(id, name, slotID, period, domain.Status(statusValue), version, revision), nil
}

func (r *Repository) CreateCreative(ctx context.Context, creative *domain.Creative) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO creatives (id, campaign_id, title, description, image_url, landing_url, status, revision)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		creative.ID(), creative.CampaignID(), creative.Title(), creative.Description(), creative.ImageURL(),
		creative.LandingURL(), string(creative.Status()), creative.Revision(),
	)
	return err
}

func (r *Repository) SaveCreative(ctx context.Context, creative *domain.Creative, expectedRevision uint64) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE creatives SET status = ?, revision = ? WHERE id = ? AND revision = ?`,
		string(creative.Status()), creative.Revision(), creative.ID(), expectedRevision,
	)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return domain.ErrConcurrentMutation
	}
	return nil
}

const creativeSelect = `
	SELECT id, campaign_id, title, description, image_url, landing_url, status, revision
	FROM creatives`

func (r *Repository) FindCreativeByID(ctx context.Context, id string) (*domain.Creative, error) {
	creative, err := scanCreative(r.db.QueryRowContext(ctx, creativeSelect+" WHERE id = ? AND status <> 'DELETED'", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrCreativeNotFound
	}
	return creative, err
}

func (r *Repository) ListCreativesByCampaign(ctx context.Context, campaignID string) ([]*domain.Creative, error) {
	rows, err := r.db.QueryContext(ctx, creativeSelect+" WHERE campaign_id = ? AND status <> 'DELETED' ORDER BY created_at DESC, id ASC", campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]*domain.Creative, 0)
	for rows.Next() {
		creative, scanErr := scanCreative(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, creative)
	}
	return result, rows.Err()
}

func (r *Repository) ListActiveCreativeIDsByCampaigns(ctx context.Context, campaignIDs []string) (map[string][]string, error) {
	result := make(map[string][]string, len(campaignIDs))
	if len(campaignIDs) == 0 {
		return result, nil
	}
	placeholders := make([]string, len(campaignIDs))
	args := make([]any, 0, len(campaignIDs)+1)
	for index, campaignID := range campaignIDs {
		placeholders[index] = "?"
		args = append(args, campaignID)
		result[campaignID] = []string{}
	}
	args = append(args, string(domain.CreativeActive))
	rows, err := r.db.QueryContext(ctx, `
		SELECT campaign_id, id
		FROM creatives
		WHERE campaign_id IN (`+strings.Join(placeholders, ",")+`) AND status = ?
		ORDER BY campaign_id ASC, created_at DESC, id ASC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var campaignID, creativeID string
		if err := rows.Scan(&campaignID, &creativeID); err != nil {
			return nil, err
		}
		result[campaignID] = append(result[campaignID], creativeID)
	}
	return result, rows.Err()
}

func scanCreative(scanner rowScanner) (*domain.Creative, error) {
	var id, campaignID, title, description, imageURL, landingURL, status string
	var revision uint64
	if err := scanner.Scan(&id, &campaignID, &title, &description, &imageURL, &landingURL, &status, &revision); err != nil {
		return nil, err
	}
	return domain.RehydrateCreative(id, campaignID, title, description, imageURL, landingURL, domain.CreativeStatus(status), revision), nil
}
