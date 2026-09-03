package migrate

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRunnerAppliesAndRecordsMigration(t *testing.T) {
	directory := t.TempDir()
	content := []byte("CREATE TABLE one (id INT);\nCREATE TABLE two (value VARCHAR(20));\n")
	path := filepath.Join(directory, "000001_test.up.sql")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT GET_LOCK(?, 10)")).WithArgs(lockName).WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(1))
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS schema_migrations").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT checksum FROM schema_migrations WHERE version = ?")).WithArgs("000001_test.up.sql").WillReturnRows(sqlmock.NewRows([]string{"checksum"}))
	mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE one (id INT)")).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE two (value VARCHAR(20))")).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO schema_migrations (version, checksum) VALUES (?, ?)")).WithArgs("000001_test.up.sql", checksum(content)).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT RELEASE_LOCK(?)")).WithArgs(lockName).WillReturnRows(sqlmock.NewRows([]string{"released"}).AddRow(1))
	runner, err := NewRunner(db, directory)
	if err != nil {
		t.Fatal(err)
	}
	applied, err := runner.Up(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 1 || applied[0] != "000001_test.up.sql" {
		t.Fatalf("applied=%v", applied)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSplitStatementsPreservesQuotedSemicolon(t *testing.T) {
	statements, err := splitStatements("INSERT INTO test(value) VALUES ('a;b'); SELECT 1;")
	if err != nil {
		t.Fatal(err)
	}
	if len(statements) != 2 || statements[0] != "INSERT INTO test(value) VALUES ('a;b')" || statements[1] != "SELECT 1" {
		t.Fatalf("statements=%q", statements)
	}
}
