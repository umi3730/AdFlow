package mysql

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
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
