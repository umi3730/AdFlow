package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/zhanghaiyang/adflow/internal/identity/domain"
)

type UserStore struct {
	users map[string]domain.User
}

func NewUserStore(spec, environment string, authEnabled bool) (*UserStore, error) {
	if strings.TrimSpace(spec) == "" {
		if authEnabled && environment != "local" && environment != "test" {
			return nil, errors.New("ADFLOW_AUTH_USERS is required outside local and test environments")
		}
		return newLocalUsers()
	}
	users := make(map[string]domain.User)
	for _, item := range strings.Split(spec, ";") {
		parts := strings.SplitN(strings.TrimSpace(item), ":", 3)
		if len(parts) != 3 {
			return nil, fmt.Errorf("invalid ADFLOW_AUTH_USERS entry")
		}
		username := strings.TrimSpace(parts[0])
		role, ok := domain.ParseRole(parts[1])
		if username == "" || !ok || parts[2] == "" {
			return nil, fmt.Errorf("invalid ADFLOW_AUTH_USERS entry")
		}
		if _, err := bcrypt.Cost([]byte(parts[2])); err != nil {
			return nil, fmt.Errorf("invalid bcrypt hash for authentication user %q", username)
		}
		key := strings.ToLower(username)
		if _, exists := users[key]; exists {
			return nil, fmt.Errorf("duplicate authentication user %q", username)
		}
		users[key] = domain.User{ID: key, Username: username, PasswordHash: parts[2], Role: role, Active: true}
	}
	return &UserStore{users: users}, nil
}

func newLocalUsers() (*UserStore, error) {
	seeds := []struct {
		username string
		password string
		role     domain.Role
	}{
		{username: "admin", password: "adflow-admin", role: domain.RoleAdmin},
		{username: "operator", password: "adflow-operator", role: domain.RoleOperator},
		{username: "viewer", password: "adflow-viewer", role: domain.RoleViewer},
	}
	users := make(map[string]domain.User, len(seeds))
	for _, seed := range seeds {
		hash, err := bcrypt.GenerateFromPassword([]byte(seed.password), bcrypt.MinCost)
		if err != nil {
			return nil, err
		}
		users[seed.username] = domain.User{ID: seed.username, Username: seed.username, PasswordHash: string(hash), Role: seed.role, Active: true}
	}
	return &UserStore{users: users}, nil
}

func (s *UserStore) FindByUsername(_ context.Context, username string) (domain.User, error) {
	user, ok := s.users[strings.ToLower(strings.TrimSpace(username))]
	if !ok {
		return domain.User{}, domain.ErrInvalidCredentials
	}
	return user, nil
}
