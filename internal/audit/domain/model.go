package domain

import (
	"context"
	"time"

	identity "github.com/umi3730/adflow/internal/identity/domain"
)

type Outcome string

const (
	OutcomeSucceeded Outcome = "SUCCEEDED"
	OutcomeFailed    Outcome = "FAILED"
)

type Entry struct {
	ID           string            `json:"id"`
	ActorID      string            `json:"actorId"`
	ActorName    string            `json:"actorName"`
	ActorRole    identity.Role     `json:"actorRole"`
	Action       string            `json:"action"`
	ResourceType string            `json:"resourceType"`
	ResourceID   string            `json:"resourceId,omitempty"`
	RequestID    string            `json:"requestId"`
	Outcome      Outcome           `json:"outcome"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	CreatedAt    time.Time         `json:"createdAt"`
}

type Filter struct {
	Limit  int
	Offset int
}

type Store interface {
	Append(context.Context, Entry) error
	List(context.Context, Filter) ([]Entry, error)
}
