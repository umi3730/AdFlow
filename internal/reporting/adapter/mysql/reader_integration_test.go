//go:build integration

package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	_ "github.com/go-sql-driver/mysql"
	dd "github.com/umi3730/adflow/internal/decision/domain"
	rd "github.com/umi3730/adflow/internal/reporting/domain"
	"os"
	"strings"
	"testing"
	"time"
)

// Temporary tables shadow persistent names on one dedicated connection, so
// this real SQL test never writes into the running application's event data.
func TestRealReportQueryTimeBucketsAndSettlementSnapshot(t *testing.T) {
	dsn := os.Getenv("ADFLOW_REPORT_IT_DSN")
	if dsn == "" {
		t.Skip("ADFLOW_REPORT_IT_DSN not configured")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	for _, query := range []string{`CREATE TEMPORARY TABLE processed_events (event_id VARCHAR(128) PRIMARY KEY, request_id VARCHAR(128),campaign_id VARCHAR(64),event_type VARCHAR(16),value_fen BIGINT,occurred_at DATETIME(3))`, `CREATE TEMPORARY TABLE event_settlements(request_id VARCHAR(128) PRIMARY KEY,event_id VARCHAR(128),status VARCHAR(16),decision_snapshot JSON)`} {
		if _, err = db.ExecContext(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	at := time.Date(2026, 9, 8, 16, 0, 0, 0, time.UTC)
	snapshot, _ := json.Marshal(dd.Result{Pricing: dd.Pricing{PriceFen: 7}})
	if !strings.Contains(string(snapshot), `"Pricing"`) {
		t.Fatal("test must use actual domain serialization")
	}
	for _, status := range []string{"SETTLED", "RECONCILE"} {
		if _, err = db.ExecContext(ctx, `INSERT INTO event_settlements VALUES (?,?,?,?)`, status, status, status, json.RawMessage(snapshot)); err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range []struct {
		id, request, campaign, kind string
		value                       int
		at                          time.Time
	}{{"SETTLED", "SETTLED", "c", "impression", 0, at}, {"click", "SETTLED", "c", "click", 0, at.Add(time.Minute)}, {"conv", "SETTLED", "c", "conversion", 300, at.Add(25 * time.Hour)}, {"RECONCILE", "RECONCILE", "c", "impression", 0, at}, {"missing", "missing", "c", "impression", 0, at}, {"other", "other", "other", "conversion", 999, at}, {"end", "SETTLED", "c", "click", 0, at.Add(48 * time.Hour)}} {
		if _, err = db.ExecContext(ctx, `INSERT INTO processed_events VALUES (?,?,?,?,?,?)`, row.id, row.request, row.campaign, row.kind, row.value, row.at); err != nil {
			t.Fatal(err)
		}
	}
	f := rd.Filter{From: at, To: at.Add(48 * time.Hour), Granularity: "day", CampaignID: "c"}
	rows, err := NewReader(db).ReadDeliveryRows(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	report := rd.Build(f, rows, at)
	if len(rows) != 2 || report.Summary.Impressions != 3 || report.Summary.Clicks != 1 || report.Summary.Conversions != 1 || report.Summary.SpendFen != 7 || report.Summary.ValueFen != 300 || report.Summary.UnpricedImpressions != 2 {
		t.Fatalf("%+v", report)
	}
	f.Granularity = "hour"
	hourly, err := NewReader(db).ReadDeliveryRows(ctx, f)
	if err != nil || len(hourly) != 2 {
		t.Fatal(hourly, err)
	}
}
