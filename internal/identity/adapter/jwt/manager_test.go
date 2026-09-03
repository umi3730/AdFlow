package jwtadapter

import (
	"errors"
	"testing"
	"time"

	"github.com/zhanghaiyang/adflow/internal/identity/domain"
)

func TestIssueAndVerify(t *testing.T) {
	manager, err := NewManager("01234567890123456789012345678901", "adflow-test", 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }
	issued, err := manager.Issue(domain.Principal{UserID: "user-1", Username: "alice", Role: domain.RoleOperator})
	if err != nil {
		t.Fatal(err)
	}
	principal, err := manager.Verify(issued.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if principal.UserID != "user-1" || principal.Username != "alice" || principal.Role != domain.RoleOperator {
		t.Fatalf("unexpected principal: %+v", principal)
	}
	if !issued.ExpiresAt.Equal(now.Add(15*time.Minute)) || issued.TokenType != "Bearer" {
		t.Fatalf("unexpected token metadata: %+v", issued)
	}
}

func TestVerifyRejectsTamperedAndExpiredTokens(t *testing.T) {
	manager, err := NewManager("01234567890123456789012345678901", "adflow-test", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }
	issued, err := manager.Issue(domain.Principal{UserID: "user-1", Username: "alice", Role: domain.RoleViewer})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Verify(issued.AccessToken + "tampered"); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("tampered token error = %v", err)
	}
	manager.now = func() time.Time { return now.Add(2 * time.Minute) }
	if _, err := manager.Verify(issued.AccessToken); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("expired token error = %v", err)
	}
}

func TestNewManagerRejectsShortSecret(t *testing.T) {
	if _, err := NewManager("short", "adflow", time.Minute); err == nil {
		t.Fatal("expected short secret to fail")
	}
}
