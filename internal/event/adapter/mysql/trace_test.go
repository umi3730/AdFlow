package mysql

import (
	"database/sql"
	"encoding/json"
	"github.com/DATA-DOG/go-sqlmock"
	decisiondomain "github.com/umi3730/adflow/internal/decision/domain"
	"testing"
	"time"
)

func TestRequestStateKeepsPublicationAndProcessingIndependent(t *testing.T) {
	db, mock := mockDB(t)
	now := time.Now().UTC()
	snapshot, _ := json.Marshal(decisiondomain.Result{RequestID: "r", Matched: true})
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT UTC_TIMESTAMP").WillReturnRows(sqlmock.NewRows([]string{"now"}).AddRow(now))
	mock.ExpectQuery("SELECT owner_id, lease_until").WithArgs("r", "r").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT decision_snapshot").WithArgs("r", "r").WillReturnRows(sqlmock.NewRows([]string{"snapshot", "status", "attempts", "accepted", "settled", "next", "lease", "error"}).AddRow(snapshot, "SETTLED", 1, now, now, now, nil, nil))
	mock.ExpectQuery("SELECT r.event_id").WithArgs("r", "r", 101).WillReturnRows(sqlmock.NewRows([]string{"id", "type", "value", "occurred", "accepted", "status", "attempts", "error", "next", "published", "processed"}).AddRow("e", "impression", 0, now, now, "PUBLISHED", 0, nil, now, now, nil))
	mock.ExpectCommit()
	state, err := NewOutbox(db, "unused").ReadRequestState(t.Context(), "r")
	if err != nil || !state.Known || state.Settlement.Status != "SETTLED" || state.Events[0].PublishedAt == nil || state.Events[0].ProcessedAt != nil {
		t.Fatalf("%+v %v", state, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
