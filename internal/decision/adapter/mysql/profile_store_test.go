package mysql

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestProfileStoreFindsProfile(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewProfileStore(db)
	mock.ExpectQuery("SELECT tags, fields").WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows([]string{"tags", "fields"}).AddRow([]byte(`["anime"]`), []byte(`{"device":"android"}`)))
	profile, err := store.FindProfile(context.Background(), "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := profile.Tags["anime"]; !ok || profile.Fields["device"] != "android" {
		t.Fatalf("unexpected profile: %+v", profile)
	}
}
