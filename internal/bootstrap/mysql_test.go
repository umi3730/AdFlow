package bootstrap

import (
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	campaignmemory "github.com/umi3730/adflow/internal/campaign/adapter/memory"
	decisionmemory "github.com/umi3730/adflow/internal/decision/adapter/memory"
)

func mysqlDemo(t *testing.T) (*DemoInitializer, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	initializer, err := NewDemoInitializer(DemoOptions{DB: db, PersistentCampaigns: true, PersistentProfiles: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
	})
	return initializer, mock
}

func expectGroupLock(mock sqlmock.Sqlmock, key string, completed any) {
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO bootstrap_seeds (seed_key) VALUES (?) ON DUPLICATE KEY UPDATE seed_key = VALUES(seed_key)")).WithArgs(key).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT completed_at FROM bootstrap_seeds WHERE seed_key = ? FOR UPDATE")).WithArgs(key).WillReturnRows(sqlmock.NewRows([]string{"completed_at"}).AddRow(completed))
}

func expectGroupComplete(mock sqlmock.Sqlmock, key string) {
	mock.ExpectExec(regexp.QuoteMeta("UPDATE bootstrap_seeds SET completed_at = UTC_TIMESTAMP(3) WHERE seed_key = ?")).WithArgs(key).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

func expectCampaignInsert(t *testing.T, mock sqlmock.Sqlmock, now time.Time) {
	t.Helper()
	fixture, err := newDemoFixture(now)
	if err != nil {
		t.Fatal(err)
	}
	c, cr := fixture.campaign, fixture.creative
	v := c.ActiveVersion()
	ruleJSON, err := json.Marshal(v.Targeting())
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec("INSERT INTO campaigns ").WithArgs(c.ID(), string(c.Name()), "ACTIVE", DemoSlotID, c.Period().Start(), c.Period().End(), uint32(1), uint64(2)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO campaign_versions ").WithArgs(DemoCampaignID, uint32(1), ruleJSON, int64(10_000), int64(1), uint32(100), now).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO creatives ").WithArgs(cr.ID(), cr.CampaignID(), cr.Title(), cr.Description(), cr.ImageURL(), cr.LandingURL(), "ACTIVE", uint64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectProfileInserts(mock sqlmock.Sqlmock) {
	mock.ExpectExec("INSERT INTO user_profiles ").WithArgs("demo-user-match", []byte(`["adflow_demo","gaming_interest"]`), []byte(`{"device":"android","score":"85"}`)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO user_profiles ").WithArgs("demo-user-excluded", []byte(`["adflow_demo","demo_excluded"]`), []byte(`{"device":"ios","score":"80"}`)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO user_profiles ").WithArgs("demo-user-miss", []byte(`["tech_interest"]`), []byte(`{"device":"android","score":"60"}`)).WillReturnResult(sqlmock.NewResult(0, 1))
}

func TestMySQLDemoInstallsCompleteGroupsAndThenSkipsBusinessWrites(t *testing.T) {
	initializer, mock := mysqlDemo(t)
	now := time.Now().UTC()
	expectGroupLock(mock, campaignSeedKey, nil)
	expectCampaignInsert(t, mock, now)
	expectGroupComplete(mock, campaignSeedKey)
	expectGroupLock(mock, profileSeedKey, nil)
	expectProfileInserts(mock)
	expectGroupComplete(mock, profileSeedKey)
	result, err := initializer.Run(t.Context(), now)
	if err != nil || result != (DemoResult{true, true}) {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	// A newly created initializer represents another process. Marker completion,
	// rather than entity existence, preserves edited and physically deleted data.
	restarted, err := NewDemoInitializer(initializer.options)
	if err != nil {
		t.Fatal(err)
	}
	expectGroupLock(mock, campaignSeedKey, now)
	mock.ExpectCommit()
	expectGroupLock(mock, profileSeedKey, now)
	mock.ExpectCommit()
	if result, err := restarted.Run(t.Context(), now); err != nil || result != (DemoResult{}) {
		t.Fatalf("repeat=%+v err=%v", result, err)
	}
}

func TestMySQLDemoRollsBackPartialCampaignAndRetries(t *testing.T) {
	initializer, mock := mysqlDemo(t)
	now := time.Now().UTC()
	failed := errors.New("version insert failed")
	expectGroupLock(mock, campaignSeedKey, nil)
	mock.ExpectExec("INSERT INTO campaigns ").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO campaign_versions ").WillReturnError(failed)
	mock.ExpectRollback()
	if _, err := initializer.Run(t.Context(), now); !errors.Is(err, failed) {
		t.Fatalf("err=%v", err)
	}
	expectGroupLock(mock, campaignSeedKey, nil)
	expectCampaignInsert(t, mock, now)
	expectGroupComplete(mock, campaignSeedKey)
	expectGroupLock(mock, profileSeedKey, nil)
	expectProfileInserts(mock)
	expectGroupComplete(mock, profileSeedKey)
	if result, err := initializer.Run(t.Context(), now); err != nil || result != (DemoResult{true, true}) {
		t.Fatalf("retry=%+v err=%v", result, err)
	}
}

func TestMySQLDemoExistingProfileIDRollsBackWithoutUpsert(t *testing.T) {
	initializer, mock := mysqlDemo(t)
	now := time.Now().UTC()
	expectGroupLock(mock, campaignSeedKey, now)
	mock.ExpectCommit()
	expectGroupLock(mock, profileSeedKey, nil)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO user_profiles (user_id, tags, fields) VALUES (?, ?, ?)")).WithArgs(DemoProfileIDs()[0], sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	conflict := errors.New("duplicate primary key")
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO user_profiles (user_id, tags, fields) VALUES (?, ?, ?)")).WithArgs(DemoProfileIDs()[1], sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnError(conflict)
	mock.ExpectRollback()
	if _, err := initializer.Run(t.Context(), now); !errors.Is(err, conflict) {
		t.Fatalf("err=%v", err)
	}
}

func TestMySQLDemoMarkerFailureRollsBackEntities(t *testing.T) {
	initializer, mock := mysqlDemo(t)
	now := time.Now().UTC()
	expectGroupLock(mock, campaignSeedKey, nil)
	expectCampaignInsert(t, mock, now)
	failed := errors.New("marker write failed")
	mock.ExpectExec("UPDATE bootstrap_seeds ").WillReturnError(failed)
	mock.ExpectRollback()
	if _, err := initializer.Run(t.Context(), now); !errors.Is(err, failed) {
		t.Fatalf("err=%v", err)
	}
}

func TestDemoMixedAdaptersSeedOnlyFreshMemoryGroup(t *testing.T) {
	for _, persistentCampaigns := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory_campaign_mysql_profiles", true: "mysql_campaign_memory_profiles"}[persistentCampaigns], func(t *testing.T) {
			base, mock := mysqlDemo(t)
			key := profileSeedKey
			if persistentCampaigns {
				key = campaignSeedKey
			}
			for range 2 {
				// Both boots get fresh memory stores and the same persistent marker.
				initializer, err := NewDemoInitializer(DemoOptions{DB: base.options.DB, Campaigns: campaignmemory.NewRepository(), Profiles: decisionmemory.NewRuntime(), PersistentCampaigns: persistentCampaigns, PersistentProfiles: !persistentCampaigns})
				if err != nil {
					t.Fatal(err)
				}
				expectGroupLock(mock, key, time.Now())
				mock.ExpectCommit()
				result, err := initializer.Run(t.Context(), time.Now())
				if err != nil || result.CampaignsInstalled == persistentCampaigns || result.ProfilesInstalled != persistentCampaigns {
					t.Fatalf("mixed=%+v err=%v", result, err)
				}
			}
		})
	}
}

func TestMySQLDemoDoesNotWriteWhenGroupLockFails(t *testing.T) {
	initializer, mock := mysqlDemo(t)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO bootstrap_seeds ").WillReturnError(sql.ErrConnDone)
	mock.ExpectRollback()
	if _, err := initializer.Run(t.Context(), time.Now()); !errors.Is(err, sql.ErrConnDone) {
		t.Fatalf("err=%v", err)
	}
}
