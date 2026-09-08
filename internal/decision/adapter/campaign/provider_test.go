package campaign

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	campaignmemory "github.com/zhanghaiyang/adflow/internal/campaign/adapter/memory"
	campaignmysql "github.com/zhanghaiyang/adflow/internal/campaign/adapter/mysql"
	campaigndomain "github.com/zhanghaiyang/adflow/internal/campaign/domain"
	decisionmemory "github.com/zhanghaiyang/adflow/internal/decision/adapter/memory"
	decisionapp "github.com/zhanghaiyang/adflow/internal/decision/application"
	decisiondomain "github.com/zhanghaiyang/adflow/internal/decision/domain"
)

var candidateTestTime = time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)

type observedCandidateRepository struct {
	*campaignmemory.Repository
	cursors       []string
	batches       [][]string
	listError     error
	failPage      int
	creativeError error
	failBatch     int
	afterList     func()
	afterBatch    func()
	ignoreCursor  bool
}

func (r *observedCandidateRepository) List(ctx context.Context, filter campaigndomain.ListFilter) ([]*campaigndomain.Campaign, error) {
	cursor := ""
	if filter.AfterID != nil {
		cursor = *filter.AfterID
	}
	r.cursors = append(r.cursors, cursor)
	if filter.Status == nil || *filter.Status != campaigndomain.StatusActive || filter.SlotID == nil || *filter.SlotID != "test-slot" {
		return nil, errors.New("candidate filter lost")
	}
	if r.listError != nil && len(r.cursors) == r.failPage {
		return nil, r.listError
	}
	if r.ignoreCursor {
		filter.AfterID = nil
	}
	rows, err := r.Repository.List(ctx, filter)
	if r.afterList != nil {
		r.afterList()
	}
	return rows, err
}

func (r *observedCandidateRepository) ListActiveCreativeIDsByCampaigns(ctx context.Context, ids []string) (map[string][]string, error) {
	r.batches = append(r.batches, append([]string(nil), ids...))
	if r.creativeError != nil && len(r.batches) == r.failBatch {
		return nil, r.creativeError
	}
	result, err := r.Repository.ListActiveCreativeIDsByCampaigns(ctx, ids)
	if r.afterBatch != nil {
		r.afterBatch()
	}
	return result, err
}

func candidatePlan(t *testing.T, index int, start, end time.Time) *campaigndomain.Campaign {
	t.Helper()
	name, err := campaigndomain.NewName("Candidate regression")
	if err != nil {
		t.Fatal(err)
	}
	slot, err := campaigndomain.NewSlotID("test-slot")
	if err != nil {
		t.Fatal(err)
	}
	period, err := campaigndomain.NewDeliveryPeriod(start, end)
	if err != nil {
		t.Fatal(err)
	}
	c := campaigndomain.NewCampaign(fmt.Sprintf("campaign-%04d", index), name, slot, period)
	rule, err := campaigndomain.NewTargetingRule([]campaigndomain.Condition{{Tag: fmt.Sprintf("audience-%04d", index)}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Publish(rule, 10000, 1, 100, candidateTestTime); err != nil {
		t.Fatal(err)
	}
	return c
}

func addCandidatePlan(t *testing.T, repo *campaignmemory.Repository, c *campaigndomain.Campaign) {
	t.Helper()
	if err := repo.Create(t.Context(), c); err != nil {
		t.Fatal(err)
	}
	creative, err := campaigndomain.NewCreative("creative-"+c.ID(), c.ID(), "Test creative", "", "https://example.com/image.png", "https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateCreative(t.Context(), creative); err != nil {
		t.Fatal(err)
	}
}

func candidateRepository(t *testing.T, count int) *observedCandidateRepository {
	t.Helper()
	repo := &observedCandidateRepository{Repository: campaignmemory.NewRepository()}
	for i := 0; i < count; i++ {
		addCandidatePlan(t, repo.Repository, candidatePlan(t, i, candidateTestTime.Add(-time.Hour), candidateTestTime.Add(time.Hour)))
	}
	return repo
}

func TestProviderReadsAllCandidatePages(t *testing.T) {
	for _, count := range []int{0, 99, 100, 101, 200, 201} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			repo := candidateRepository(t, count)
			result, err := NewProvider(repo, repo).ActiveCandidates(t.Context(), "test-slot", candidateTestTime)
			if err != nil || len(result) != count {
				t.Fatalf("count=%d got=%d err=%v", count, len(result), err)
			}
			seen := make(map[string]bool)
			for _, c := range result {
				if seen[c.CampaignID] || len(c.CreativeIDs) != 1 || c.CreativeIDs[0] != "creative-"+c.CampaignID {
					t.Fatalf("invalid candidate: %+v", c)
				}
				seen[c.CampaignID] = true
			}
			if len(repo.cursors) != count/100+1 {
				t.Fatalf("page cursors=%v", repo.cursors)
			}
			for i, cursor := range repo.cursors {
				expected := ""
				if i > 0 {
					expected = fmt.Sprintf("campaign-%04d", i*100-1)
				}
				if cursor != expected {
					t.Fatalf("page cursors=%v", repo.cursors)
				}
			}
			if len(repo.batches) != (count+99)/100 {
				t.Fatalf("creative calls=%d", len(repo.batches))
			}
			for _, batch := range repo.batches {
				if len(batch) == 0 || len(batch) > 100 {
					t.Fatalf("unbounded/empty creative batch: %d", len(batch))
				}
			}
		})
	}
}

func TestDecisionCanMatchThe201stCandidate(t *testing.T) {
	repo := candidateRepository(t, 201)
	runtime := decisionmemory.NewRuntime()
	if err := runtime.PutProfile(t.Context(), decisiondomain.NewProfile("tail-user", []string{"audience-0200"}, nil)); err != nil {
		t.Fatal(err)
	}
	source, err := NewCachedProvider(NewProvider(repo, repo), time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	engine := decisionapp.NewService(source, runtime, runtime, runtime, runtime)
	result, err := engine.Decide(t.Context(), decisiondomain.Request{RequestID: "tail-request", UserID: "tail-user", SlotID: "test-slot", Now: candidateTestTime})
	if err != nil || !result.Matched || result.CampaignID != "campaign-0200" {
		t.Fatalf("tail result=%+v err=%v", result, err)
	}
}

func TestProviderDoesNotStopAtAnIneligiblePage(t *testing.T) {
	repo := candidateRepository(t, 0)
	for i := 0; i < 100; i++ {
		start, end := candidateTestTime.Add(-time.Hour), candidateTestTime
		if i%2 == 1 {
			start, end = candidateTestTime.Add(time.Hour), candidateTestTime.Add(2*time.Hour)
		}
		addCandidatePlan(t, repo.Repository, candidatePlan(t, i, start, end))
	}
	addCandidatePlan(t, repo.Repository, candidatePlan(t, 100, candidateTestTime, candidateTestTime.Add(time.Hour)))
	result, err := NewProvider(repo, repo).ActiveCandidates(t.Context(), "test-slot", candidateTestTime)
	if err != nil || len(result) != 1 || result[0].CampaignID != "campaign-0100" {
		t.Fatalf("candidates=%+v err=%v", result, err)
	}
	if len(repo.batches) != 1 || len(repo.batches[0]) != 1 {
		t.Fatalf("ineligible IDs queried: %v", repo.batches)
	}
}

func TestProviderDiscardsFailedPagesAndCacheRetriesFromTheBeginning(t *testing.T) {
	for _, stage := range []string{"campaign", "creative"} {
		t.Run(stage, func(t *testing.T) {
			repo := candidateRepository(t, 101)
			failure := errors.New("later page unavailable")
			if stage == "campaign" {
				repo.listError, repo.failPage = failure, 2
			} else {
				repo.creativeError, repo.failBatch = failure, 2
			}
			source, err := NewCachedProvider(NewProvider(repo, repo), time.Hour, nil)
			if err != nil {
				t.Fatal(err)
			}
			result, err := source.ActiveCandidates(t.Context(), "test-slot", candidateTestTime)
			if !errors.Is(err, failure) || len(result) != 0 {
				t.Fatalf("partial result cached/returned: count=%d err=%v", len(result), err)
			}
			repo.listError, repo.creativeError = nil, nil
			repo.cursors, repo.batches = nil, nil
			result, err = source.ActiveCandidates(t.Context(), "test-slot", candidateTestTime)
			if err != nil || len(result) != 101 || len(repo.cursors) != 2 || repo.cursors[0] != "" {
				t.Fatalf("retry count=%d cursors=%v err=%v", len(result), repo.cursors, err)
			}
		})
	}
}

func TestProviderHonorsCancellationWithoutReturningPartialCandidates(t *testing.T) {
	for _, stage := range []string{"before", "after-list", "after-creatives"} {
		t.Run(stage, func(t *testing.T) {
			repo := candidateRepository(t, 101)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			switch stage {
			case "before":
				cancel()
			case "after-list":
				repo.afterList = cancel
			case "after-creatives":
				repo.afterBatch = cancel
			}
			result, err := NewProvider(repo, repo).ActiveCandidates(ctx, "test-slot", candidateTestTime)
			if !errors.Is(err, context.Canceled) || result != nil {
				t.Fatalf("result=%v err=%v", result, err)
			}
			if stage == "before" && len(repo.cursors) != 0 {
				t.Fatal("queried canceled request")
			}
			if stage == "after-list" && len(repo.batches) != 0 {
				t.Fatal("loaded creatives after cancellation")
			}
			if len(repo.cursors) > 1 {
				t.Fatal("loaded another page after cancellation")
			}
		})
	}
}

func TestProviderMySQLReadsTheSecondPageWithBatchedCreatives(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := campaignmysql.NewRepository(db)
	columns := []string{"id", "name", "slot_id", "start_at", "end_at", "status", "revision", "version", "targeting_rule", "daily_budget", "impression_cost", "frequency_limit", "published_at", "auction_terms"}
	for _, offset := range []int{0, 100} {
		rows := sqlmock.NewRows(columns)
		creatives := sqlmock.NewRows([]string{"campaign_id", "id"})
		var args []driver.Value
		for i := offset; i < min(offset+100, 101); i++ {
			c := candidatePlan(t, i, candidateTestTime.Add(-time.Hour), candidateTestTime.Add(time.Hour))
			v := c.ActiveVersion()
			rule, err := json.Marshal(v.Targeting())
			if err != nil {
				t.Fatal(err)
			}
			rows.AddRow(c.ID(), string(c.Name()), string(c.SlotID()), c.Period().Start(), c.Period().End(), string(c.Status()), c.Revision(), v.Number(), rule, v.DailyBudget().Amount(), v.ImpressionCost().Amount(), v.FrequencyLimit(), v.PublishedAt(), nil)
			creatives.AddRow(c.ID(), "creative-"+c.ID())
			args = append(args, c.ID())
		}
		cursor := ""
		if offset > 0 {
			cursor = "campaign-0099"
		}
		mock.ExpectQuery(`(?s)FROM campaigns.*c.id > \?.*ORDER BY c.id ASC LIMIT \? OFFSET \?`).WithArgs("ACTIVE", "test-slot", cursor, 100, 0).WillReturnRows(rows).RowsWillBeClosed()
		args = append(args, "ACTIVE")
		mock.ExpectQuery(`(?s)SELECT campaign_id, id.*FROM creatives.*WHERE campaign_id IN`).WithArgs(args...).WillReturnRows(creatives).RowsWillBeClosed()
	}
	result, err := NewProvider(repo, repo).ActiveCandidates(t.Context(), "test-slot", candidateTestTime)
	if err != nil || len(result) != 101 || result[100].CreativeIDs[0] != "creative-campaign-0100" {
		t.Fatalf("count=%d err=%v", len(result), err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProviderDeduplicatesRowsThatMoveBetweenPages(t *testing.T) {
	repo := candidateRepository(t, 101)
	repo.afterList = func() {
		repo.afterList = nil
		addCandidatePlan(t, repo.Repository, candidatePlan(t, -1, candidateTestTime.Add(-time.Hour), candidateTestTime.Add(time.Hour)))
	}
	result, err := NewProvider(repo, repo).ActiveCandidates(t.Context(), "test-slot", candidateTestTime)
	if err != nil || len(result) != 101 {
		t.Fatalf("count=%d err=%v", len(result), err)
	}
	seen := make(map[string]bool)
	for _, c := range result {
		if seen[c.CampaignID] {
			t.Fatalf("duplicate %s", c.CampaignID)
		}
		seen[c.CampaignID] = true
	}
	if !seen["campaign-0100"] {
		t.Fatal("tail candidate missing")
	}
}

func TestProviderCursorKeepsTailWhenEarlierPlansAreRemoved(t *testing.T) {
	for _, remove := range []bool{false, true} {
		t.Run(fmt.Sprintf("delete=%t", remove), func(t *testing.T) {
			repo := candidateRepository(t, 101)
			repo.afterList = func() {
				repo.afterList = nil
				c, err := repo.FindByID(t.Context(), "campaign-0000")
				if err != nil {
					t.Fatal(err)
				}
				revision := c.Revision()
				if err := c.Pause(candidateTestTime); err != nil {
					t.Fatal(err)
				}
				if remove {
					if err := c.Delete(); err != nil {
						t.Fatal(err)
					}
				}
				if err := repo.Save(t.Context(), c, revision); err != nil {
					t.Fatal(err)
				}
			}
			result, err := NewProvider(repo, repo).ActiveCandidates(t.Context(), "test-slot", candidateTestTime)
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range result {
				if c.CampaignID == "campaign-0100" {
					return
				}
			}
			t.Fatal("an earlier removal shifted the tail out of the scan")
		})
	}
}

func TestProviderRejectsANonAdvancingCursor(t *testing.T) {
	repo := candidateRepository(t, 101)
	repo.ignoreCursor = true
	result, err := NewProvider(repo, repo).ActiveCandidates(t.Context(), "test-slot", candidateTestTime)
	if err == nil || result != nil || len(repo.cursors) != 2 {
		t.Fatalf("count=%d calls=%d err=%v", len(result), len(repo.cursors), err)
	}
}
