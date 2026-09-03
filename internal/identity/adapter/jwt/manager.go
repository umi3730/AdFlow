package jwtadapter

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/zhanghaiyang/adflow/internal/identity/domain"
)

type claims struct {
	Username string      `json:"username"`
	Role     domain.Role `json:"role"`
	jwt.RegisteredClaims
}

type Manager struct {
	secret []byte
	issuer string
	ttl    time.Duration
	now    func() time.Time
}

func NewManager(secret, issuer string, ttl time.Duration) (*Manager, error) {
	if len(secret) < 32 {
		return nil, errors.New("JWT secret must contain at least 32 characters")
	}
	if issuer == "" || ttl <= 0 {
		return nil, errors.New("JWT issuer and positive TTL are required")
	}
	return &Manager{secret: []byte(secret), issuer: issuer, ttl: ttl, now: time.Now}, nil
}

func (m *Manager) Issue(principal domain.Principal) (domain.IssuedToken, error) {
	now := m.now().UTC()
	expiresAt := now.Add(m.ttl)
	tokenID, err := randomID()
	if err != nil {
		return domain.IssuedToken{}, fmt.Errorf("create token id: %w", err)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
		Username: principal.Username,
		Role:     principal.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   principal.UserID,
			ID:        tokenID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	})
	signed, err := token.SignedString(m.secret)
	if err != nil {
		return domain.IssuedToken{}, fmt.Errorf("sign access token: %w", err)
	}
	return domain.IssuedToken{AccessToken: signed, TokenType: "Bearer", ExpiresAt: expiresAt, Principal: principal}, nil
}

func (m *Manager) Verify(raw string) (domain.Principal, error) {
	parsedClaims := &claims{}
	token, err := jwt.ParseWithClaims(raw, parsedClaims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, domain.ErrInvalidToken
		}
		return m.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer(m.issuer), jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithTimeFunc(m.now))
	if err != nil || !token.Valid || parsedClaims.Subject == "" || parsedClaims.Username == "" {
		return domain.Principal{}, domain.ErrInvalidToken
	}
	role, ok := domain.ParseRole(string(parsedClaims.Role))
	if !ok {
		return domain.Principal{}, domain.ErrInvalidToken
	}
	return domain.Principal{UserID: parsedClaims.Subject, Username: parsedClaims.Username, Role: role}, nil
}

func randomID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}
