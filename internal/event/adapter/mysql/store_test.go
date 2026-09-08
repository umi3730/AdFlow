package mysql

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/umi3730/adflow/internal/event/domain"
)

func TestStoreRecordUpdatesMetricsInSameTransaction(t *testing.T) {
	db, mock := mockDB(t)
	store := NewStore(db)
	event := sampleEvent()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT IGNORE INTO processed_events").
		WithArgs(event.EventID, event.RequestID, event.CampaignID, event.CreativeID, string(event.Type), event.ValueFen, event.OccurredAt).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO campaign_metrics").
		WithArgs(event.CampaignID, uint64(1), uint64(0), uint64(0), int64(0)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	created, err := store.Record(context.Background(), event)
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
}

func TestStoreRecordBatchUsesOneTransactionAndAggregatesMetrics(t *testing.T) {
	db, mock := mockDB(t)
	store := NewStore(db)
	first := sampleEvent()
	second := first
	second.EventID = "event-2"
	second.Type = domain.Conversion
	second.ValueFen = 500

	mock.ExpectBegin()
	mock.ExpectExec("INSERT IGNORE INTO processed_events").
		WithArgs(
			first.EventID, first.RequestID, first.CampaignID, first.CreativeID, string(first.Type), first.ValueFen, first.OccurredAt, sqlmock.AnyArg(),
			second.EventID, second.RequestID, second.CampaignID, second.CreativeID, string(second.Type), second.ValueFen, second.OccurredAt, sqlmock.AnyArg(),
		).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectQuery("SELECT event_id, campaign_id, event_type, value_fen").WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"event_id", "campaign_id", "event_type", "value_fen"}).
			AddRow(first.EventID, first.CampaignID, string(first.Type), first.ValueFen).
			AddRow(second.EventID, second.CampaignID, string(second.Type), second.ValueFen))
	mock.ExpectExec("INSERT INTO campaign_metrics").
		WithArgs(first.CampaignID, uint64(1), uint64(0), uint64(1), int64(500)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	created, err := store.RecordBatch(context.Background(), []domain.Event{first, second})
	if err != nil || len(created) != 2 || !created[0] || !created[1] {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStoreHasImpressionsUsesOneQuery(t *testing.T) {
	db, mock := mockDB(t)
	store := NewStore(db)
	mock.ExpectQuery("SELECT DISTINCT request_id FROM processed_events").
		WithArgs("request-1", "request-2").
		WillReturnRows(sqlmock.NewRows([]string{"request_id"}).AddRow("request-2"))

	result, err := store.HasImpressions(context.Background(), []string{"request-1", "request-2", "request-1"})
	if err != nil || result["request-1"] || !result["request-2"] {
		t.Fatalf("result=%v err=%v", result, err)
	}
}
