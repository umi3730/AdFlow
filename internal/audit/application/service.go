package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/umi3730/adflow/internal/audit/domain"
	identity "github.com/umi3730/adflow/internal/identity/domain"
)

type RecordCommand struct {
	Principal    identity.Principal
	Action       string
	ResourceType string
	ResourceID   string
	RequestID    string
	Outcome      domain.Outcome
	Metadata     map[string]string
}

type Service struct {
	store domain.Store
	now   func() time.Time
}

func NewService(store domain.Store) *Service {
	return &Service{store: store, now: time.Now}
}

func (s *Service) Record(ctx context.Context, command RecordCommand) error {
	id, err := newID()
	if err != nil {
		return fmt.Errorf("create audit id: %w", err)
	}
	entry := domain.Entry{
		ID: id, ActorID: command.Principal.UserID, ActorName: command.Principal.Username,
		ActorRole: command.Principal.Role, Action: command.Action, ResourceType: command.ResourceType,
		ResourceID: command.ResourceID, RequestID: command.RequestID, Outcome: command.Outcome,
		Metadata: command.Metadata, CreatedAt: s.now().UTC(),
	}
	return s.store.Append(ctx, entry)
}

func (s *Service) List(ctx context.Context, filter domain.Filter) ([]domain.Entry, error) {
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 50
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	return s.store.List(ctx, filter)
}

func newID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}
