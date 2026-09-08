package memory

import (
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/umi3730/adflow/internal/identity/domain"
)

func TestLocalUsers(t *testing.T) {
	store, err := NewUserStore("", "test", true)
	if err != nil {
		t.Fatal(err)
	}
	user, err := store.FindByUsername(t.Context(), "ADMIN")
	if err != nil {
		t.Fatal(err)
	}
	if user.Role != domain.RoleAdmin || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("adflow-admin")) != nil {
		t.Fatalf("unexpected local user: %+v", user)
	}
}

func TestConfiguredUsers(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("safe-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewUserStore("alice:viewer:"+string(hash), "production", true)
	if err != nil {
		t.Fatal(err)
	}
	user, err := store.FindByUsername(t.Context(), "alice")
	if err != nil || user.Role != domain.RoleViewer {
		t.Fatalf("user=%+v err=%v", user, err)
	}
	if _, err := NewUserStore("alice:viewer:"+string(hash)+";alice:admin:"+string(hash), "production", true); err == nil {
		t.Fatal("expected duplicate user to fail")
	}
}

func TestProductionAuthRequiresUsers(t *testing.T) {
	if _, err := NewUserStore("", "production", true); err == nil {
		t.Fatal("expected missing production users to fail")
	}
}
