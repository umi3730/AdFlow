package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/umi3730/adflow/internal/identity/domain"
)

var registrationUsername = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{2,31}$`)

// Configure once before serving. This demo intentionally registers admins.
func (s *Service) EnableRegistration(hasher domain.PasswordHasher) {
	s.hasher = hasher
	s.registrationSlots = make(chan struct{}, 4)
}

func (s *Service) RegistrationEnabled() bool {
	_, ok := s.users.(domain.UserCreator)
	return ok && s.hasher != nil
}

func (s *Service) Register(ctx context.Context, username, password string) (domain.IssuedToken, error) {
	if !s.RegistrationEnabled() {
		return domain.IssuedToken{}, domain.ErrRegistrationDisabled
	}
	username = strings.ToLower(strings.TrimSpace(username))
	if !registrationUsername.MatchString(username) || utf8.RuneCountInString(password) < 8 || len(password) > 72 || !utf8.ValidString(password) {
		return domain.IssuedToken{}, domain.ErrInvalidRegistration
	}
	if err := ctx.Err(); err != nil {
		return domain.IssuedToken{}, err
	}
	select {
	case s.registrationSlots <- struct{}{}:
		defer func() { <-s.registrationSlots }()
	default:
		return domain.IssuedToken{}, domain.ErrRegistrationBusy
	}
	if _, err := s.users.FindByUsername(ctx, username); err == nil {
		return domain.IssuedToken{}, domain.ErrUsernameTaken
	} else if !errors.Is(err, domain.ErrInvalidCredentials) {
		return domain.IssuedToken{}, err
	}
	hash, err := s.hasher.Hash(password)
	if err != nil {
		return domain.IssuedToken{}, err
	}
	if err := ctx.Err(); err != nil {
		return domain.IssuedToken{}, err
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return domain.IssuedToken{}, err
	}
	user := domain.User{ID: hex.EncodeToString(id[:]), Username: username, PasswordHash: hash, Role: domain.RoleAdmin, Active: true}
	// Issue before insertion: a signer failure must not leave an unusable account.
	token, err := s.tokens.Issue(domain.Principal{UserID: user.ID, Username: user.Username, Role: user.Role})
	if err != nil {
		return domain.IssuedToken{}, err
	}
	if err := s.users.(domain.UserCreator).Create(ctx, user); err != nil {
		return domain.IssuedToken{}, err
	}
	return token, nil
}
