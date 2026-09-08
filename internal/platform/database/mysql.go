package database

import (
	"database/sql"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
)

func OpenMySQL(dsn string) (*sql.DB, error) {
	// DATETIME values, SQL defaults and UTC lease comparisons must share one
	// clock representation. loc=UTC alone only controls driver conversion.
	cfg, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		return nil, err
	}
	cfg.Loc = time.UTC
	// These statements are executed once through database/sql. Let the driver
	// encode parameters rather than preparing, executing and closing a server
	// statement on every call; this removes extra network round trips. The
	// driver retains its charset/SQL-mode-aware escaping and fallback rules.
	cfg.InterpolateParams = true
	if cfg.Params == nil {
		cfg.Params = make(map[string]string)
	}
	cfg.Params["time_zone"] = "'+00:00'"
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(30)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(3 * time.Minute)
	db.SetConnMaxIdleTime(1 * time.Minute)
	return db, nil
}
