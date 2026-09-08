package mysql

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	driver "github.com/go-sql-driver/mysql"
	"github.com/umi3730/adflow/internal/identity/adapter/memory"
	"github.com/umi3730/adflow/internal/identity/domain"
)

func TestPersistentUsersAndReservedSeeds(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	seeds, _ := memory.NewUserStore("", "test", true)
	s := NewUserStore(db, seeds)
	if user, err := s.FindByUsername(t.Context(), "ADMIN"); err != nil || user.Role != domain.RoleAdmin {
		t.Fatal(err)
	}
	if err := s.Create(t.Context(), domain.User{Username: "admin"}); !errors.Is(err, domain.ErrUsernameTaken) {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT id,username,password_hash,role,active FROM auth_users").WithArgs("alice").WillReturnError(sql.ErrNoRows)
	if _, err := s.FindByUsername(t.Context(), "ALICE"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatal(err)
	}
	user := domain.User{ID: "123", Username: "Alice", PasswordHash: "hash", Role: domain.RoleAdmin, Active: true}
	mock.ExpectExec("INSERT INTO auth_users").WithArgs("123", "alice", "hash", domain.RoleAdmin, true).WillReturnError(&driver.MySQLError{Number: 1062})
	if err := s.Create(t.Context(), user); !errors.Is(err, domain.ErrUsernameTaken) {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
