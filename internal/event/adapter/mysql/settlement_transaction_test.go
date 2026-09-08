package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	driver "github.com/go-sql-driver/mysql"
	decisiondomain "github.com/umi3730/adflow/internal/decision/domain"
	"github.com/umi3730/adflow/internal/event/domain"
)

func retryEntry() domain.SettlementEntry {
	return domain.SettlementEntry{Owner: "owner", Settlement: decisiondomain.Settlement{EventID: "event", Decision: decisiondomain.Result{RequestID: "request"}}}
}
func TestSettlementCompletionRetriesWholeRolledBackTransaction(t *testing.T) {
	for _, code := range []uint16{1213, 1205} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			db, mock := mockDB(t)
			mock.ExpectBegin()
			mock.ExpectExec("UPDATE event_settlements").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("UPDATE event_outbox").WillReturnError(fmt.Errorf("wrapped: %w", &driver.MySQLError{Number: code}))
			mock.ExpectRollback()
			mock.ExpectBegin()
			mock.ExpectExec("UPDATE event_settlements").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("UPDATE event_outbox").WillReturnResult(sqlmock.NewResult(0, 3))
			mock.ExpectCommit()
			if err := NewOutbox(db, "worker").CompleteSettlement(t.Context(), retryEntry()); err != nil {
				t.Fatal(err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestSettlementFailureRetriesWithoutDoubleIncrement(t *testing.T) {
	db, mock := mockDB(t)
	for attempt := 0; attempt < 2; attempt++ {
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE event_settlements SET status").WithArgs("PENDING", int64(1000000), "temporary", "request", "event", "owner").WillReturnResult(sqlmock.NewResult(0, 1))
		q := mock.ExpectExec("UPDATE event_outbox SET status").WithArgs("SETTLING", 1, "temporary", int64(1000000), "request")
		if attempt == 0 {
			q.WillReturnError(&driver.MySQLError{Number: 1213})
			mock.ExpectRollback()
		} else {
			q.WillReturnResult(sqlmock.NewResult(0, 3))
			mock.ExpectCommit()
		}
	}
	if err := NewOutbox(db, "worker").FailSettlement(t.Context(), retryEntry(), "temporary", time.Second, false); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestSettlementCommitUncertaintyDoesNotRetry(t *testing.T) {
	db, mock := mockDB(t)
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE event_settlements").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE event_outbox").WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectCommit().WillReturnError(io.ErrUnexpectedEOF)
	if err := NewOutbox(db, "worker").CompleteSettlement(t.Context(), retryEntry()); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestSettlementRetriesAreBounded(t *testing.T) {
	db, mock := mockDB(t)
	deadlock := &driver.MySQLError{Number: 1213}
	for range 3 {
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE event_settlements").WillReturnError(deadlock)
		mock.ExpectRollback()
	}
	if err := NewOutbox(db, "worker").CompleteSettlement(t.Context(), retryEntry()); !errors.Is(err, deadlock) {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestSettlementRetryStopsOnCancellation(t *testing.T) {
	db, mock := mockDB(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	mock.ExpectBegin()
	mock.ExpectRollback()
	err := NewOutbox(db, "worker").settlementTransaction(ctx, func(_ *sql.Tx) error { cancel(); return &driver.MySQLError{Number: 1213} })
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestReleaseSettlementClaimsIsFencedAndIdempotent(t *testing.T) {
	db, mock := mockDB(t)
	entries := []domain.SettlementEntry{retryEntry(), retryEntry()}
	entries[0].Settlement.Decision.RequestID = "z"
	entries[0].Settlement.EventID = "last"
	entries[1].Settlement.Decision.RequestID = "a"
	entries[1].Settlement.EventID = "first"
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE event_settlements.*WHERE request_id = .*event_id = .*status = 'PROCESSING' AND locked_by =").WithArgs("a", "first", "owner").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE event_settlements.*WHERE request_id = .*event_id = .*status = 'PROCESSING' AND locked_by =").WithArgs("z", "last", "owner").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := NewOutbox(db, "worker").ReleaseSettlements(t.Context(), entries); err != nil {
		t.Fatal(err)
	}
	if entries[0].Settlement.Decision.RequestID != "z" {
		t.Fatal("mutated caller batch")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestReleaseSettlementRejectsMissingOwner(t *testing.T) {
	db, mock := mockDB(t)
	entry := retryEntry()
	entry.Owner = ""
	if err := NewOutbox(db, "worker").ReleaseSettlements(t.Context(), []domain.SettlementEntry{entry}); !errors.Is(err, domain.ErrSettlementLeaseLost) {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSettlementIngressRetriesReceiptAndOutboxTogether(t *testing.T) {
	db, mock := mockDB(t)
	event := sampleEvent()
	for attempt := 0; attempt < 2; attempt++ {
		mock.ExpectBegin()
		mock.ExpectExec("INSERT IGNORE INTO event_settlements").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectQuery("SELECT event_id, status.*FOR UPDATE").WillReturnRows(sqlmock.NewRows([]string{"event_id", "status"}).AddRow(event.EventID, "PENDING"))
		mock.ExpectExec("INSERT IGNORE INTO event_receipts").WillReturnResult(sqlmock.NewResult(0, 1))
		write := mock.ExpectExec("INSERT INTO event_outbox")
		if attempt == 0 {
			write.WillReturnError(&driver.MySQLError{Number: 1213})
			mock.ExpectRollback()
		} else {
			write.WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
		}
	}
	created, err := NewOutbox(db, "worker").EnqueueForSettlement(t.Context(), event, decisiondomain.Result{}, event.OccurredAt)
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPublishedBatchRetriesBothTables(t *testing.T) {
	db, mock := mockDB(t)
	for attempt := 0; attempt < 2; attempt++ {
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE event_outbox").WillReturnResult(sqlmock.NewResult(0, 2))
		write := mock.ExpectExec("UPDATE event_receipts")
		if attempt == 0 {
			write.WillReturnError(&driver.MySQLError{Number: 1213})
			mock.ExpectRollback()
		} else {
			write.WillReturnResult(sqlmock.NewResult(0, 2))
			mock.ExpectCommit()
		}
	}
	if err := NewOutbox(db, "worker").MarkPublishedBatch(t.Context(), []string{"one", "two"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
