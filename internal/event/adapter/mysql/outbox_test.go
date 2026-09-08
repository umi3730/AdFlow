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
	now := time.Date(2026, 9, 3, 1, 0, 0, 123456789, time.UTC)
	databaseNow := now.Truncate(time.Millisecond)
	lockedUntil := databaseNow.Add(30 * time.Second)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT UTC_TIMESTAMP").WillReturnRows(sqlmock.NewRows([]string{"now"}).AddRow(databaseNow))
	mock.ExpectQuery("SELECT event_id, payload, attempts.*FOR UPDATE SKIP LOCKED").
		WithArgs(databaseNow, databaseNow, 10).
		WillReturnRows(sqlmock.NewRows([]string{"event_id", "payload", "attempts"}).AddRow(event.EventID, payload, 2))
	mock.ExpectExec("UPDATE event_outbox.*WHERE event_id IN").WithArgs("worker-1", lockedUntil, event.EventID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	entries, err := outbox.ClaimBatch(context.Background(), 10, now, 30*time.Second)
	if err != nil || len(entries) != 1 || entries[0].Attempts != 2 || entries[0].Event.EventID != event.EventID {
		t.Fatalf("entries=%+v err=%v", entries, err)
	}
}

func TestOutboxMarksPublishedBatchAndDependencyReceiptsAtomically(t *testing.T) {
	db, mock := mockDB(t)
	outbox := NewOutbox(db, "worker-1")
	publishedAt := time.Date(2026, 9, 3, 1, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE event_outbox").
		WithArgs(publishedAt, "event-1", "event-2", "worker-1").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("UPDATE event_receipts SET status = 'PUBLISHED'").WithArgs("event-1", "event-2").WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()
	if err := outbox.MarkPublishedBatch(context.Background(), []string{"event-1", "event-2"}, publishedAt); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestOutboxReplaysDeadLetter(t *testing.T) {
	db, mock := mockDB(t)
	outbox := NewOutbox(db, "worker-1")
	now := time.Date(2026, 9, 3, 1, 0, 0, 0, time.UTC)
	mock.ExpectExec("UPDATE event_outbox").WithArgs(now, "event-1").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := outbox.ReplayDeadLetter(context.Background(), "event-1", now); err != nil {
		t.Fatal(err)
	}
}

func TestOutboxListsOperationalRecords(t *testing.T) {
	db, mock := mockDB(t)
	outbox := NewOutbox(db, "worker-1")
	event := sampleEvent()
	payload, _ := json.Marshal(event)
	now := time.Date(2026, 9, 3, 1, 0, 0, 0, time.UTC)
	mock.ExpectQuery("FROM event_outbox WHERE status =").WithArgs("DEAD_LETTERED", 50, 0).
		WillReturnRows(sqlmock.NewRows([]string{"payload", "status", "attempts", "next_attempt_at", "locked_by", "locked_until", "published_at", "dead_lettered_at", "last_error", "created_at"}).
			AddRow(payload, "DEAD_LETTERED", 8, now, nil, nil, nil, now, "broker unavailable", now))
	records, err := outbox.ListOutbox(context.Background(), domain.OutboxFilter{Status: "DEAD_LETTERED", Limit: 50})
	if err != nil || len(records) != 1 || records[0].Event.EventID != event.EventID || records[0].LastError != "broker unavailable" || records[0].DeadLetteredAt == nil {
		t.Fatalf("records=%+v err=%v", records, err)
	}
}
