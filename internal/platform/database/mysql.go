package database

import (
	"database/sql"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func OpenMySQL(dsn string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(30)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(3 * time.Minute)
	db.SetConnMaxIdleTime(1 * time.Minute)
	return db, nil
}
