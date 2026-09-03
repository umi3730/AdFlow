package migrate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const lockName = "adflow_schema_migrations"

type Runner struct {
	db  *sql.DB
	dir string
}

func NewRunner(db *sql.DB, dir string) (*Runner, error) {
	if db == nil || strings.TrimSpace(dir) == "" {
		return nil, errors.New("migration database and directory are required")
	}
	return &Runner{db: db, dir: dir}, nil
}

func (r *Runner) Up(ctx context.Context) ([]string, error) {
	connection, err := r.db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("open migration connection: %w", err)
	}
	defer connection.Close()
	if err := acquireLock(ctx, connection); err != nil {
		return nil, err
	}
	defer releaseLock(connection)
	if _, err := connection.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) NOT NULL,
			checksum CHAR(64) NOT NULL,
			applied_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
			PRIMARY KEY (version)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`); err != nil {
		return nil, fmt.Errorf("create migration history: %w", err)
	}
	files, err := filepath.Glob(filepath.Join(r.dir, "*.up.sql"))
	if err != nil {
		return nil, fmt.Errorf("list migrations: %w", err)
	}
	sort.Strings(files)
	applied := make([]string, 0, len(files))
	for _, path := range files {
		name := filepath.Base(path)
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", name, err)
		}
		checksum := checksum(content)
		var existing string
		err = connection.QueryRowContext(ctx, `SELECT checksum FROM schema_migrations WHERE version = ?`, name).Scan(&existing)
		if err == nil {
			if existing != checksum {
				return nil, fmt.Errorf("migration %s checksum changed after application", name)
			}
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("read migration history for %s: %w", name, err)
		}
		statements, err := splitStatements(string(content))
		if err != nil {
			return nil, fmt.Errorf("parse migration %s: %w", name, err)
		}
		for index, statement := range statements {
			if _, err := connection.ExecContext(ctx, statement); err != nil {
				return nil, fmt.Errorf("apply migration %s statement %d: %w", name, index+1, err)
			}
		}
		if _, err := connection.ExecContext(ctx, `INSERT INTO schema_migrations (version, checksum) VALUES (?, ?)`, name, checksum); err != nil {
			return nil, fmt.Errorf("record migration %s: %w", name, err)
		}
		applied = append(applied, name)
	}
	return applied, nil
}

func acquireLock(ctx context.Context, connection *sql.Conn) error {
	var acquired int
	if err := connection.QueryRowContext(ctx, `SELECT GET_LOCK(?, 10)`, lockName).Scan(&acquired); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	if acquired != 1 {
		return errors.New("migration lock was not acquired")
	}
	return nil
}

func releaseLock(connection *sql.Conn) {
	var released sql.NullInt64
	_ = connection.QueryRowContext(context.Background(), `SELECT RELEASE_LOCK(?)`, lockName).Scan(&released)
}

func checksum(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func splitStatements(content string) ([]string, error) {
	var statements []string
	var current strings.Builder
	var quote rune
	escaped := false
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if quote == 0 && strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		for _, char := range line + "\n" {
			if escaped {
				current.WriteRune(char)
				escaped = false
				continue
			}
			if quote != 0 {
				current.WriteRune(char)
				if char == '\\' && quote != '`' {
					escaped = true
				} else if char == quote {
					quote = 0
				}
				continue
			}
			switch char {
			case '\'', '"', '`':
				quote = char
				current.WriteRune(char)
			case ';':
				if statement := strings.TrimSpace(current.String()); statement != "" {
					statements = append(statements, statement)
				}
				current.Reset()
			default:
				current.WriteRune(char)
			}
		}
	}
	if quote != 0 {
		return nil, errors.New("unterminated quoted string")
	}
	if statement := strings.TrimSpace(current.String()); statement != "" {
		statements = append(statements, statement)
	}
	return statements, nil
}
