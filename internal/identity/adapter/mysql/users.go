package mysql

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	driver "github.com/go-sql-driver/mysql"
	"github.com/umi3730/adflow/internal/identity/domain"
)

// Configured seed accounts remain authoritative and cannot be registered over.
type UserStore struct {
	db    *sql.DB
	seeds domain.UserRepository
}

func NewUserStore(db *sql.DB, seeds domain.UserRepository) *UserStore {
	return &UserStore{db: db, seeds: seeds}
}

func (s *UserStore) FindByUsername(ctx context.Context, username string) (domain.User, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if s.seeds != nil {
		user, err := s.seeds.FindByUsername(ctx, username)
		if err == nil {
			return user, nil
		}
		if !errors.Is(err, domain.ErrInvalidCredentials) {
			return domain.User{}, err
		}
	}
	var user domain.User
	err := s.db.QueryRowContext(ctx, `SELECT id,username,password_hash,role,active FROM auth_users WHERE username=?`, username).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.Active)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, domain.ErrInvalidCredentials
	}
	return user, err
}

func (s *UserStore) Create(ctx context.Context, user domain.User) error {
	user.Username = strings.ToLower(strings.TrimSpace(user.Username))
	if s.seeds != nil {
		if _, err := s.seeds.FindByUsername(ctx, user.Username); err == nil {
			return domain.ErrUsernameTaken
		} else if !errors.Is(err, domain.ErrInvalidCredentials) {
			return err
		}
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO auth_users (id,username,password_hash,role,active) VALUES (?,?,?,?,?)`, user.ID, user.Username, user.PasswordHash, user.Role, user.Active)
	var mysqlErr *driver.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return domain.ErrUsernameTaken
	}
	return err
}
