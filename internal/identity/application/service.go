package application

import (
	"context"
	"strings"

	"github.com/umi3730/adflow/internal/identity/domain"
)

type Service struct {
	users             domain.UserRepository
	passwords         domain.PasswordVerifier
	tokens            domain.TokenManager
	hasher            domain.PasswordHasher
	registrationSlots chan struct{}
}

func NewService(users domain.UserRepository, passwords domain.PasswordVerifier, tokens domain.TokenManager) *Service {
	return &Service{users: users, passwords: passwords, tokens: tokens}
}

func (s *Service) Login(ctx context.Context, username, password string) (domain.IssuedToken, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return domain.IssuedToken{}, domain.ErrInvalidCredentials
	}
	user, err := s.users.FindByUsername(ctx, username)
	if err != nil || s.passwords.Compare(user.PasswordHash, password) != nil {
		return domain.IssuedToken{}, domain.ErrInvalidCredentials
	}
	if !user.Active {
		return domain.IssuedToken{}, domain.ErrInactiveUser
	}
	return s.tokens.Issue(domain.Principal{UserID: user.ID, Username: user.Username, Role: user.Role})
}

func (s *Service) Authenticate(_ context.Context, token string) (domain.Principal, error) {
	return s.tokens.Verify(token)
}

func (s *Service) Authorize(principal domain.Principal, method, route string) bool {
	required, ok := requiredRole(method, route)
	return ok && principal.Role.Allows(required)
}
