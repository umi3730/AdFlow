//go:build integration

package mysql

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"github.com/umi3730/adflow/internal/identity/adapter/jwt"
	"github.com/umi3730/adflow/internal/identity/adapter/memory"
	"github.com/umi3730/adflow/internal/identity/adapter/password"
	"github.com/umi3730/adflow/internal/identity/application"
	"github.com/umi3730/adflow/internal/identity/domain"
	"github.com/umi3730/adflow/internal/platform/database"
	"github.com/umi3730/adflow/internal/platform/migrate"
)

func TestMySQLRegistrationSurvivesNewServiceAndConcurrentDuplicates(t *testing.T) {
	dsn := os.Getenv("ADFLOW_AUTH_TEST_DSN")
	if dsn == "" {
		t.Skip("requires isolated auth test database")
	}
	cfg, err := driver.ParseDSN(dsn)
	if err != nil || !strings.HasPrefix(cfg.DBName, "adflow_auth_it_") {
		t.Fatal("isolated auth database required")
	}
	db, err := database.OpenMySQL(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	runner, err := migrate.NewRunner(db, filepath.Join("..", "..", "..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = runner.Up(t.Context()); err != nil {
		t.Fatal(err)
	}
	seeds, _ := memory.NewUserStore("", "test", true)
	tokens, _ := jwtadapter.NewManager("01234567890123456789012345678901", "test", time.Hour)
	first := application.NewService(NewUserStore(db, seeds), password.Bcrypt{}, tokens)
	first.EnableRegistration(password.Bcrypt{})
	name := fmt.Sprintf("persist-%d", time.Now().UnixNano())
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM auth_users WHERE username IN (?,?)", name, name+"-race") })
	created, err := first.Register(t.Context(), name, "registered-password")
	if err != nil {
		t.Fatal(err)
	}
	secondDB, err := database.OpenMySQL(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer secondDB.Close()
	secondSeeds, _ := memory.NewUserStore("", "test", true)
	second := application.NewService(NewUserStore(secondDB, secondSeeds), password.Bcrypt{}, tokens)
	second.EnableRegistration(password.Bcrypt{})
	loggedIn, err := second.Login(t.Context(), strings.ToUpper(name), "registered-password")
	if err != nil || loggedIn.Principal.UserID != created.Principal.UserID || loggedIn.Principal.Role != domain.RoleAdmin {
		t.Fatal("persistent login failed", err)
	}
	var hash string
	if err = db.QueryRow("SELECT password_hash FROM auth_users WHERE username=?", name).Scan(&hash); err != nil || hash == "registered-password" || (password.Bcrypt{}).Compare(hash, "registered-password") != nil {
		t.Fatal("stored password is not a bcrypt hash", err)
	}
	if _, err = second.Register(t.Context(), strings.ToUpper(name), "another-password"); !errors.Is(err, domain.ErrUsernameTaken) {
		t.Fatal(err)
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s := first
			if i%2 != 0 {
				s = second
			}
			_, err := s.Register(t.Context(), name+"-race", "registered-password")
			if err == nil {
				successes.Add(1)
			} else if !errors.Is(err, domain.ErrUsernameTaken) && !errors.Is(err, domain.ErrRegistrationBusy) {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("concurrent registrations created %d accounts", successes.Load())
	}
}
