//go:build integration

package database

import (
	"encoding/json"
	"os"
	"testing"
)

// Read-only check against an explicitly supplied connection. Quoted SQL-like
// strings, UTF-8 and binary bytes must remain values with interpolation enabled.
func TestMySQLParameterValuesAndSessionClock(t *testing.T) {
	dsn := os.Getenv("ADFLOW_MYSQL_PROBE_DSN")
	if dsn == "" {
		t.Skip("set ADFLOW_MYSQL_PROBE_DSN for the read-only real-driver check")
	}
	db, err := OpenMySQL(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, want := range []string{"中文标签", "quote' double\" slash\\ question?", "'; SELECT 99; --", "binary\x00line\nend"} {
		var got string
		var number int
		if err := db.QueryRowContext(t.Context(), "SELECT ?, ?", want, 42).Scan(&got, &number); err != nil {
			t.Fatal(err)
		}
		if got != want || number != 42 {
			t.Fatal("parameter did not round trip as a literal")
		}
	}
	var zone string
	var tag string
	const jsonValue = `{"tag":"中文 ' quote"}`
	if err := db.QueryRowContext(t.Context(), `SELECT JSON_UNQUOTE(JSON_EXTRACT(CAST(? AS JSON), '$.tag'))`, json.RawMessage(jsonValue)).Scan(&tag); err != nil {
		t.Fatal(err)
	}
	if tag != "中文 ' quote" {
		t.Fatal("JSON text did not retain its character encoding")
	}
	if err := db.QueryRowContext(t.Context(), "SELECT @@session.time_zone").Scan(&zone); err != nil {
		t.Fatal(err)
	}
	if zone != "+00:00" {
		t.Fatalf("session zone=%q", zone)
	}
}
