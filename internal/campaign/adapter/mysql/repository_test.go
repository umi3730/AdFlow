package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/zhanghaiyang/adflow/internal/campaign/domain"
)

func testRepository(t *testing.T) (*Repository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewRepository(db), mock
}

func draftCampaign(t *testing.T) *domain.Campaign {
	t.Helper()
	name, _ := domain.NewName("Strategy Campaign")
	slot, _ := domain.NewSlotID("game-home-banner")
	period, _ := domain.NewDeliveryPeriod(time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC))
	return domain.NewCampaign("campaign000000000000000000000001", name, slot, period)
}

func TestCreateCampaign(t *testing.T) {
	repository, mock := testRepository(t)
	campaign := draftCampaign(t)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO campaigns (id, name, status, slot_id, start_at, end_at, active_version, revision)")).
		WithArgs(campaign.ID(), string(campaign.Name()), string(campaign.Status()), string(campaign.SlotID()), campaign.Period().Start(), campaign.Period().End(), campaign.Revision()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := repository.Create(context.Background(), campaign); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSavePublishedCampaignUsesTransactionAndRevision(t *testing.T) {
	repository, mock := testRepository(t)
	campaign := draftCampaign(t)
	rule, _ := domain.NewTargetingRule([]domain.Condition{{Tag: "anime"}}, nil, nil)
	publishedAt := time.Date(2026, 9, 2, 1, 0, 0, 0, time.UTC)
	if err := campaign.Publish(rule, 10_000, 100, 3, publishedAt); err != nil {
		t.Fatal(err)
	}
	targeting, _ := json.Marshal(campaign.ActiveVersion().Targeting())

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO campaign_versions").
		WithArgs(campaign.ID(), uint32(1), targeting, int64(10_000), int64(100), uint32(3), publishedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE campaigns").
		WithArgs(string(campaign.Name()), string(campaign.Status()), string(campaign.SlotID()), campaign.Period().Start(), campaign.Period().End(), uint32(1), campaign.Revision(), campaign.ID(), uint64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := repository.Save(context.Background(), campaign, 1); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFindCampaignRehydratesAggregate(t *testing.T) {
	repository, mock := testRepository(t)
	start := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	published := start.Add(time.Hour)
	rows := sqlmock.NewRows([]string{
		"id", "name", "slot_id", "start_at", "end_at", "status", "revision",
		"version", "targeting_rule", "daily_budget", "impression_cost", "frequency_limit", "published_at",
	}).AddRow(
		"campaign000000000000000000000001", "Strategy Campaign", "game-home-banner", start, end, "ACTIVE", 2,
		1, []byte(`{"all":[{"tag":"anime"}]}`), 10_000, 100, 3, published,
	)
	mock.ExpectQuery("SELECT c.id").WithArgs("campaign000000000000000000000001").WillReturnRows(rows)

	campaign, err := repository.FindByID(context.Background(), "campaign000000000000000000000001")
	if err != nil {
		t.Fatal(err)
	}
	if campaign.Status() != domain.StatusActive || campaign.ActiveVersion() == nil || campaign.ActiveVersion().Number() != 1 {
		t.Fatalf("unexpected campaign: status=%s version=%v", campaign.Status(), campaign.ActiveVersion())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFindCampaignMapsNoRows(t *testing.T) {
	repository, mock := testRepository(t)
	mock.ExpectQuery("SELECT c.id").WithArgs("missing").WillReturnError(sql.ErrNoRows)
	_, err := repository.FindByID(context.Background(), "missing")
	if err != domain.ErrCampaignNotFound {
		t.Fatalf("error = %v", err)
	}
}

func TestListCampaignsFiltersByStatusAndSlot(t *testing.T) {
	repository, mock := testRepository(t)
	status := domain.StatusActive
	slot, _ := domain.NewSlotID("game-home-banner")
	start := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	rows := sqlmock.NewRows([]string{
		"id", "name", "slot_id", "start_at", "end_at", "status", "revision",
		"version", "targeting_rule", "daily_budget", "impression_cost", "frequency_limit", "published_at",
	}).AddRow(
		"campaign000000000000000000000001", "Strategy Campaign", string(slot), start, end, string(status), 2,
		1, []byte(`{"all":[{"tag":"anime"}]}`), 10_000, 100, 3, start.Add(time.Hour),
	)
	mock.ExpectQuery(regexp.QuoteMeta("WHERE c.status = ? AND c.slot_id = ? ORDER BY c.created_at DESC, c.id ASC LIMIT ? OFFSET ?")).
		WithArgs(string(status), string(slot), 100, 0).WillReturnRows(rows)

	campaigns, err := repository.List(context.Background(), domain.ListFilter{Status: &status, SlotID: &slot, Limit: 100})
	if err != nil || len(campaigns) != 1 || campaigns[0].SlotID() != slot {
		t.Fatalf("campaigns=%v err=%v", campaigns, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListActiveCreativeIDsByCampaignsUsesOneQuery(t *testing.T) {
	repository, mock := testRepository(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT campaign_id, id FROM creatives WHERE campaign_id IN (?,?) AND status = ? ORDER BY campaign_id ASC, created_at DESC, id ASC")).
		WithArgs("campaign-1", "campaign-2", string(domain.CreativeActive)).
		WillReturnRows(sqlmock.NewRows([]string{"campaign_id", "id"}).
			AddRow("campaign-1", "creative-1").
			AddRow("campaign-2", "creative-2"))

	result, err := repository.ListActiveCreativeIDsByCampaigns(context.Background(), []string{"campaign-1", "campaign-2"})
	if err != nil || len(result["campaign-1"]) != 1 || result["campaign-1"][0] != "creative-1" || len(result["campaign-2"]) != 1 {
		t.Fatalf("result=%v err=%v", result, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
