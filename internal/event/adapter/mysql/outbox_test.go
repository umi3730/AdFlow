package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/zhanghaiyang/adflow/internal/event/domain"
)

func mockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, mock
}

func sampleEvent() domain.Event {
	return domain.Event{
		EventID: "event-1", RequestID: "request-1", CampaignID: "campaign000000000000000000000001",
		CreativeID: "creative000000000000000000000001", Type: domain.Impression,
		OccurredAt: time.Date(2026, 9, 3, 1, 0, 0, 0, time.UTC),
	}
}

func TestOutboxEnqueueWritesReceiptAndMessageAtomically(t *testing.T) {
	db, mock := mockDB(t)
	outbox := NewOutbox(db, "worker-1")
	event := sampleEvent()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT IGNORE INTO event_receipts").
		WithArgs(event.EventID, event.RequestID, event.CampaignID, event.CreativeID, string(event.Type), event.ValueFen, event.OccurredAt).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO event_outbox").
		WithArgs(event.EventID, event.RequestID, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	created, err := outbox.Enqueue(context.Background(), event)
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestOutboxEnqueueIsIdempotent(t *testing.T) {
	db, mock := mockDB(t)
	outbox := NewOutbox(db, "worker-1")
	event := sampleEvent()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT IGNORE INTO event_receipts").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	created, err := outbox.Enqueue(context.Background(), event)
	if err != nil || created {
		t.Fatalf("created=%v err=%v", created, err)
	}
}

func TestOutboxClaimBatchUsesLease(t *testing.T) {
	db, mock := mockDB(t)
	outbox := NewOutbox(db, "worker-1")
	event := sampleEvent()
	payload, _ := json.Marshal(event)
	now := time.Date(2026, 9, 3, 1, 0, 0, 0, time.UTC)
	lockedUntil := now.Add(30 * time.Second)
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE event_outbox").
		WithArgs("worker-1", lockedUntil, now, now, 10).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT payload, attempts").
		WithArgs("worker-1", lockedUntil).
		WillReturnRows(sqlmock.NewRows([]string{"payload", "attempts"}).AddRow(payload, 2))
	mock.ExpectCommit()
	entries, err := outbox.ClaimBatch(context.Background(), 10, now, 30*time.Second)
	if err != nil || len(entries) != 1 || entries[0].Attempts != 2 || entries[0].Event.EventID != event.EventID {
		t.Fatalf("entries=%+v err=%v", entries, err)
	}
}
