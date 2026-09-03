package domain

import (
	"context"
	"errors"
	"strings"
	"time"
)

type Role string

const (
	RoleViewer   Role = "viewer"
	RoleOperator Role = "operator"
	RoleAdmin    Role = "admin"
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrInvalidToken       = errors.New("invalid access token")
	ErrInactiveUser       = errors.New("user is inactive")
)

func ParseRole(value string) (Role, bool) {
	role := Role(strings.ToLower(strings.TrimSpace(value)))
	switch role {
	case RoleViewer, RoleOperator, RoleAdmin:
		return role, true
	default:
		return "", false
	}
}

func (r Role) Allows(required Role) bool {
	return roleRank(r) >= roleRank(required)
}

func roleRank(role Role) int {
	switch role {
	case RoleAdmin:
		return 3
	case RoleOperator:
		return 2
	case RoleViewer:
		return 1
	default:
		return 0
	}
}

type User struct {
	ID           string
	Username     string
	PasswordHash string
	Role         Role
	Active       bool
}

type Principal struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
	Role     Role   `json:"role"`
}

type IssuedToken struct {
	AccessToken string    `json:"accessToken"`
	TokenType   string    `json:"tokenType"`
	ExpiresAt   time.Time `json:"expiresAt"`
	Principal   Principal `json:"principal"`
}

type UserRepository interface {
	FindByUsername(context.Context, string) (User, error)
}

type PasswordVerifier interface {
	Compare(hash, password string) error
}

type TokenManager interface {
	Issue(Principal) (IssuedToken, error)
	Verify(string) (Principal, error)
}
