package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	"github.com/zhanghaiyang/adflow/internal/platform/database"
	"github.com/zhanghaiyang/adflow/internal/platform/migrate"
)

func main() {
	dsn := flag.String("dsn", environmentOr("ADFLOW_MYSQL_DSN", "adflow:adflow@tcp(127.0.0.1:3306)/adflow?parseTime=true&charset=utf8mb4&loc=Local"), "MySQL DSN")
	directory := flag.String("dir", "migrations", "migration directory")
	timeout := flag.Duration("timeout", 30*time.Second, "migration timeout")
	flag.Parse()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	db, err := database.OpenMySQL(*dsn)
	if err != nil {
		logger.Error("open MySQL", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		logger.Error("ping MySQL", "error", err)
		os.Exit(1)
	}
	runner, err := migrate.NewRunner(db, *directory)
	if err != nil {
		logger.Error("create migration runner", "error", err)
		os.Exit(1)
	}
	applied, err := runner.Up(ctx)
	if err != nil {
		logger.Error("apply migrations", "error", err)
		os.Exit(1)
	}
	logger.Info("migrations complete", "applied", applied, "count", len(applied))
}

func environmentOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
