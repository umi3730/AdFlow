package application

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/zhanghaiyang/adflow/internal/identity/domain"
)

type fakeUsers struct{ user domain.User }

func (f fakeUsers) FindByUsername(context.Context, string) (domain.User, error) { return f.user, nil }

type fakePasswords struct{ err error }

func (f fakePasswords) Compare(string, string) error { return f.err }

type fakeTokens struct{}

func (fakeTokens) Issue(principal domain.Principal) (domain.IssuedToken, error) {
	return domain.IssuedToken{AccessToken: "token", TokenType: "Bearer", ExpiresAt: time.Now(), Principal: principal}, nil
}

func (fakeTokens) Verify(string) (domain.Principal, error) {
	return domain.Principal{UserID: "1", Username: "alice", Role: domain.RoleOperator}, nil
}

func TestLoginUsesGenericCredentialError(t *testing.T) {
	service := NewService(fakeUsers{user: domain.User{ID: "1", Username: "alice", PasswordHash: "hash", Role: domain.RoleOperator, Active: true}}, fakePasswords{err: errors.New("mismatch")}, fakeTokens{})
	if _, err := service.Login(context.Background(), "alice", "wrong-password"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v", err)
	}
}

func TestLoginReturnsPrincipalInToken(t *testing.T) {
	service := NewService(fakeUsers{user: domain.User{ID: "1", Username: "alice", PasswordHash: "hash", Role: domain.RoleOperator, Active: true}}, fakePasswords{}, fakeTokens{})
	token, err := service.Login(context.Background(), "alice", "correct-password")
	if err != nil {
		t.Fatal(err)
	}
	if token.Principal.Role != domain.RoleOperator || token.Principal.Username != "alice" {
		t.Fatalf("unexpected token: %+v", token)
	}
}

func TestRBACPolicy(t *testing.T) {
	service := NewService(nil, nil, nil)
	viewer := domain.Principal{Role: domain.RoleViewer}
	operator := domain.Principal{Role: domain.RoleOperator}
	admin := domain.Principal{Role: domain.RoleAdmin}
	if !service.Authorize(viewer, http.MethodGet, "/v1/campaigns") {
		t.Fatal("viewer should read campaigns")
	}
	if service.Authorize(viewer, http.MethodPost, "/v1/campaigns") {
		t.Fatal("viewer should not create campaigns")
	}
	if !service.Authorize(operator, http.MethodPost, "/v1/campaigns") {
		t.Fatal("operator should create campaigns")
	}
	if service.Authorize(operator, http.MethodPost, "/v1/campaigns/:id/publish") {
		t.Fatal("operator should not publish campaigns")
	}
	if !service.Authorize(admin, http.MethodPost, "/v1/campaigns/:id/publish") {
		t.Fatal("admin should publish campaigns")
	}
	if service.Authorize(operator, http.MethodGet, "/v1/audit-logs") {
		t.Fatal("only admin should read audit logs")
	}
	if service.Authorize(operator, http.MethodPost, "/v1/operations/dead-letters/:eventId/replay") || !service.Authorize(admin, http.MethodPost, "/v1/operations/dead-letters/:eventId/replay") {
		t.Fatal("only admin should replay dead letters")
	}
}
