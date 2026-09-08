package mysql

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/umi3730/adflow/internal/decision/domain"
)

func TestProfileDeletionUsesBoundID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectExec("DELETE FROM user_profiles WHERE user_id = ").WithArgs("user-1").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := NewProfileStore(db).DeleteProfile(t.Context(), "user-1"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

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

func TestProfileStoreListsWithBoundParameters(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT COUNT.*LOCATE.*JSON_CONTAINS.*JSON_EXTRACT").WithArgs("user", "anime", "android").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT user_id, tags, fields.*LIMIT.*OFFSET").WithArgs("user", "anime", "android", 20, 0).WillReturnRows(sqlmock.NewRows([]string{"user_id", "tags", "fields"}).AddRow("user-1", `["anime"]`, `{"device":"android","custom":"preserved"}`))
	page, err := NewProfileStore(db).ListProfiles(t.Context(), domain.ProfileFilter{Query: "user", Tag: "anime", Device: "android", Limit: 20})
	if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].Fields["custom"] != "preserved" {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
