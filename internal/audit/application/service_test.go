package application

import (
	"context"
	"testing"
	"time"

	"github.com/zhanghaiyang/adflow/internal/audit/domain"
	identity "github.com/zhanghaiyang/adflow/internal/identity/domain"
)

type recordingStore struct {
	entry  domain.Entry
	filter domain.Filter
}

func (s *recordingStore) Append(_ context.Context, entry domain.Entry) error {
	s.entry = entry
	return nil
}

func (s *recordingStore) List(_ context.Context, filter domain.Filter) ([]domain.Entry, error) {
	s.filter = filter
	return []domain.Entry{s.entry}, nil
}

func TestRecordAndList(t *testing.T) {
	store := &recordingStore{}
	service := NewService(store)
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	err := service.Record(t.Context(), RecordCommand{
		Principal: identity.Principal{UserID: "admin-1", Username: "admin", Role: identity.RoleAdmin},
		Action:    "PUBLISH_CAMPAIGN", ResourceType: "campaign", ResourceID: "campaign-1",
		RequestID: "request-1", Outcome: domain.OutcomeSucceeded,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(store.entry.ID) != 32 || store.entry.CreatedAt != now || store.entry.ActorRole != identity.RoleAdmin {
		t.Fatalf("unexpected entry: %+v", store.entry)
	}
	if _, err := service.List(t.Context(), domain.Filter{Limit: 1000, Offset: -1}); err != nil {
		t.Fatal(err)
	}
	if store.filter.Limit != 50 || store.filter.Offset != 0 {
		t.Fatalf("unexpected normalized filter: %+v", store.filter)
	}
}
