package main

import (
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
	"time"
)

func TestDrainObservesBatchCommittedBetweenReads(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	prefix := "kc-test_%"
	first := "kc-test_1700000000000_0"
	last := "kc-test_1700000000001_1"
	at := time.Now().UTC()
	observeQuery := "SELECT request_id,MAX\\(processed_at\\) FROM processed_events"
	mock.ExpectQuery(observeQuery).WithArgs(prefix).WillReturnRows(sqlmock.NewRows([]string{"request_id", "max"}).AddRow(first, at))
	// A concurrent transaction completes the second request before the count read.
	mock.ExpectQuery("SELECT\\s+\\(SELECT COUNT\\(\\*\\) FROM decisions").WithArgs(prefix, prefix, prefix, prefix, prefix, prefix).WillReturnRows(sqlmock.NewRows([]string{"decisions", "receipts", "settled", "reconcile", "published", "processed"}).AddRow(2, 6, 2, 0, 6, 6))
	mock.ExpectQuery(observeQuery).WithArgs(prefix).WillReturnRows(sqlmock.NewRows([]string{"request_id", "max"}).AddRow(first, at).AddRow(last, at))
	h := &chainCapacity{db: db}
	seen := map[string]chainObservation{first: {RequestID: first, ObservedMS: 123}}
	var timeline []chainSnapshot
	_, drained, err := h.drainObserved(prefix, seen, 2, &timeline)
	if err != nil || !drained {
		t.Fatalf("drained=%v error=%v", drained, err)
	}
	if len(seen) != 2 || seen[last].RequestID != last {
		t.Fatalf("missing final observation: %+v", seen)
	}
	if seen[first].ObservedMS != 123 {
		t.Fatal("existing observation overwritten")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
