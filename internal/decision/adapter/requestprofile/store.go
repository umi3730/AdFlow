package requestprofile

import (
	"context"
	"github.com/zhanghaiyang/adflow/internal/decision/domain"
)

type profileKey struct{}
type Store struct{ source domain.ProfileStore }

func New(source domain.ProfileStore) *Store { return &Store{source: source} }

func clone(profile domain.Profile) domain.Profile {
	tags := make([]string, 0, len(profile.Tags))
	for tag := range profile.Tags {
		tags = append(tags, tag)
	}
	return domain.NewProfile(profile.UserID, tags, profile.Fields)
}

// The override is request-scoped only: it never enters MySQL, Redis or the catalog.
func WithProfile(ctx context.Context, profile domain.Profile) context.Context {
	return context.WithValue(ctx, profileKey{}, clone(profile))
}

func (s *Store) FindProfile(ctx context.Context, userID string) (domain.Profile, error) {
	if profile, ok := ctx.Value(profileKey{}).(domain.Profile); ok && profile.UserID == userID {
		return clone(profile), nil
	}
	return s.source.FindProfile(ctx, userID)
}
func (s *Store) PutProfile(ctx context.Context, profile domain.Profile) error {
	return s.source.PutProfile(ctx, profile)
}
