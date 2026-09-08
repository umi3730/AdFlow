//go:build integration

package mysql

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"github.com/zhanghaiyang/adflow/internal/platform/database"
)

func TestRealMySQLDeadlockRetriesCompleteTransaction(t *testing.T) {
	dsn := os.Getenv("ADFLOW_SETTLEMENT_TEST_DSN")
	if dsn == "" {
		t.Skip("requires isolated MySQL test DSN")
	}
	cfg, err := driver.ParseDSN(dsn)
	if err != nil || !strings.HasPrefix(cfg.DBName, "adflow_settlement_it_") {
		t.Fatal("requires dedicated adflow_settlement_it_ schema")
	}
	db, err := database.OpenMySQL(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(4)
	if _, err = db.Exec("CREATE TABLE settlement_retry_probe (id INT PRIMARY KEY, n INT NOT NULL) ENGINE=InnoDB"); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP TABLE settlement_retry_probe")
	for i := 1; i <= 12; i++ {
		if _, err = db.Exec("INSERT INTO settlement_retry_probe VALUES (?,0)", i); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	firstLocked, secondLocked := make(chan struct{}), make(chan struct{})
	other := make(chan error, 1)
	go func() {
		tx, e := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
		if e != nil {
			other <- e
			return
		}
		defer tx.Rollback()
		// More undo/locks than the retrying transaction, so InnoDB selects the small
		// transaction as the deadlock victim.
		if _, e = tx.ExecContext(ctx, "UPDATE settlement_retry_probe SET n=n+1 WHERE id BETWEEN 2 AND 12"); e != nil {
			other <- e
			return
		}
		close(secondLocked)
		select {
		case <-firstLocked:
		case <-ctx.Done():
			other <- ctx.Err()
			return
		}
		if _, e = tx.ExecContext(ctx, "UPDATE settlement_retry_probe SET n=n+1 WHERE id=1"); e != nil {
			other <- e
			return
		}
		other <- tx.Commit()
	}()
	attempts := 0
	err = NewOutbox(db, "retry").settlementTransaction(ctx, func(tx *sql.Tx) error {
		attempts++
		if _, e := tx.ExecContext(ctx, "UPDATE settlement_retry_probe SET n=n+1 WHERE id=1"); e != nil {
			return e
		}
		if attempts == 1 {
			close(firstLocked)
			select {
			case <-secondLocked:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		_, e := tx.ExecContext(ctx, "UPDATE settlement_retry_probe SET n=n+1 WHERE id=2")
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = <-other; err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("expected one real deadlock and retry, attempts=%d", attempts)
	}
	for i := 1; i <= 12; i++ {
		var n int
		if err = db.QueryRow("SELECT n FROM settlement_retry_probe WHERE id=?", i).Scan(&n); err != nil {
			t.Fatal(err)
		}
		want := 1
		if i <= 2 {
			want = 2
		}
		if n != want {
			t.Fatalf("partial transaction was not rolled back: id=%d value=%d want=%d", i, n, want)
		}
	}
}
