package application

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/umi3730/adflow/internal/identity/adapter/memory"
	"github.com/umi3730/adflow/internal/identity/adapter/password"
	"github.com/umi3730/adflow/internal/identity/domain"
)

func TestRegisterPersistsHashAndGrantsDemoAdmin(t *testing.T) {
	users, _ := memory.NewUserStore("", "test", true)
	s := NewService(users, password.Bcrypt{}, fakeTokens{})
	if _, err := s.Register(t.Context(), "alice", "demo-password"); !errors.Is(err, domain.ErrRegistrationDisabled) {
		t.Fatal(err)
	}
	s.EnableRegistration(password.Bcrypt{})
	token, err := s.Register(t.Context(), " Alice ", "demo-password")
	if err != nil || token.Principal.Role != domain.RoleAdmin || token.Principal.Username != "alice" {
		t.Fatalf("registration: %v %v", token.Principal, err)
	}
	user, err := users.FindByUsername(t.Context(), "ALICE")
	if err != nil || user.PasswordHash == "demo-password" || (password.Bcrypt{}).Compare(user.PasswordHash, "demo-password") != nil {
		t.Fatal("password not hashed correctly")
	}
	if _, err = s.Login(t.Context(), "ALICE", "demo-password"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Register(t.Context(), "alice", "another-password"); !errors.Is(err, domain.ErrUsernameTaken) {
		t.Fatal(err)
	}
	if _, err = s.Register(t.Context(), "ADMIN", "another-password"); !errors.Is(err, domain.ErrUsernameTaken) {
		t.Fatal("seed account overwritten", err)
	}
	if _, err = s.Login(t.Context(), "admin", "adflow-admin"); err != nil {
		t.Fatal(err)
	}
	for _, input := range [][2]string{{"ab", "demo-password"}, {"bad name", "demo-password"}, {"_user", "demo-password"}, {"valid-user", "short"}, {"valid-user", strings.Repeat("中", 25)}} {
		if _, err = s.Register(t.Context(), input[0], input[1]); !errors.Is(err, domain.ErrInvalidRegistration) {
			t.Fatal("invalid input accepted", err)
		}
	}
}

func TestConcurrentRegistrationCreatesOneUsername(t *testing.T) {
	users, _ := memory.NewUserStore("", "test", true)
	s := NewService(users, password.Bcrypt{}, fakeTokens{})
	s.EnableRegistration(password.Bcrypt{})
	var successes atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Register(t.Context(), "same-user", "demo-password")
			if err == nil {
				successes.Add(1)
			} else if !errors.Is(err, domain.ErrUsernameTaken) && !errors.Is(err, domain.ErrRegistrationBusy) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("created %d users", successes.Load())
	}
}
