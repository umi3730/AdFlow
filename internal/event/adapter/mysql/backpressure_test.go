package mysql

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestReadDecisionBacklogUsesCappedActiveRows(t *testing.T) {
	db, mock := mockDB(t)
	outbox := NewOutbox(db, "test")
	mock.ExpectQuery("SELECT.*SELECT COUNT.*event_settlements WHERE status IN.*LIMIT.*event_outbox WHERE status IN.*LIMIT").
		WithArgs(int64(100), int64(600)).WillReturnRows(sqlmock.NewRows([]string{"settlements", "outbox"}).AddRow(100, 400))
	got, err := outbox.ReadDecisionBacklog(t.Context(), 100, 600)
	if err != nil || got.Settlements != 100 || got.Outbox != 400 {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := outbox.ReadDecisionBacklog(t.Context(), 0, 600); err == nil {
		t.Fatal("accepted invalid cap")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
