package profilecache

import (
	"context"
	"errors"
	"github.com/zhanghaiyang/adflow/internal/decision/domain"
	"sync"
	"testing"
	"time"
)

type pausedSnapshot struct {
	*fakeProfiles
	captured, resume chan struct{}
	once             sync.Once
}

func (p *pausedSnapshot) FindProfile(ctx context.Context, id string) (domain.Profile, error) {
	value, err := p.fakeProfiles.FindProfile(ctx, id)
	p.once.Do(func() {
		close(p.captured)
		select {
		case <-p.resume:
		case <-ctx.Done():
		}
	})
	return value, err
}
func secondStore(t *testing.T, first *Store, source domain.ProfileStore) *Store {
	t.Helper()
	s, err := New(source, first.client, "adflow:test:profile", first.ttl, first.negativeTTL, time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestCrossInstanceStaleRefillAfterMutation(t *testing.T) {
	for _, mutation := range []string{"update", "delete", "recreate", "create-after-negative-read", "generation-expired"} {
		t.Run(mutation, func(t *testing.T) {
			source := &fakeProfiles{profiles: map[string]domain.Profile{}}
			if mutation != "create-after-negative-read" {
				source.profiles["u"] = domain.NewProfile("u", nil, map[string]string{"score": "10"})
			}
			paused := &pausedSnapshot{fakeProfiles: source, captured: make(chan struct{}), resume: make(chan struct{})}
			reader, server := testStore(t, paused)
			reader.timeout = time.Second
			writer := secondStore(t, reader, source)
			done := make(chan struct{})
			go func() { defer close(done); _, _ = reader.FindProfile(t.Context(), "u") }()
			<-paused.captured
			if mutation == "generation-expired" {
				server.FastForward(3 * time.Minute)
			}
			if mutation == "delete" || mutation == "recreate" {
				if err := writer.DeleteProfile(t.Context(), "u"); err != nil {
					t.Fatal(err)
				}
			}
			if mutation != "delete" {
				if err := writer.PutProfile(t.Context(), domain.NewProfile("u", nil, map[string]string{"score": "99"})); err != nil {
					t.Fatal(err)
				}
			}
			close(paused.resume)
			<-done
			for range 2 {
				got, err := writer.FindProfile(t.Context(), "u")
				if mutation == "delete" {
					if !errors.Is(err, domain.ErrProfileNotFound) {
						t.Fatalf("deleted profile returned: %+v %v", got, err)
					}
				} else if err != nil || got.Fields["score"] != "99" {
					t.Fatalf("stale refill: %+v %v", got, err)
				}
			}
		})
	}
}

type pausedWrite struct {
	*fakeProfiles
	committed, resume chan struct{}
}

func (p *pausedWrite) PutProfile(ctx context.Context, profile domain.Profile) error {
	err := p.fakeProfiles.PutProfile(ctx, profile)
	close(p.committed)
	<-p.resume
	return err
}
func TestLateWriterCompletionDoesNotCacheOlderInput(t *testing.T) {
	source := &fakeProfiles{profiles: map[string]domain.Profile{}}
	paused := &pausedWrite{fakeProfiles: source, committed: make(chan struct{}), resume: make(chan struct{})}
	first, _ := testStore(t, paused)
	second := secondStore(t, first, source)
	done := make(chan error, 1)
	go func() {
		done <- first.PutProfile(t.Context(), domain.NewProfile("u", nil, map[string]string{"score": "10"}))
	}()
	<-paused.committed
	if err := second.PutProfile(t.Context(), domain.NewProfile("u", nil, map[string]string{"score": "99"})); err != nil {
		t.Fatal(err)
	}
	if _, err := second.FindProfile(t.Context(), "u"); err != nil {
		t.Fatal(err)
	}
	close(paused.resume)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	got, err := second.FindProfile(t.Context(), "u")
	if err != nil || got.Fields["score"] != "99" {
		t.Fatalf("late writer overwrote cache: %+v %v", got, err)
	}
}

func TestMutationReportsFailedInvalidation(t *testing.T) {
	source := &fakeProfiles{profiles: map[string]domain.Profile{}}
	s, server := testStore(t, source)
	server.Close()
	err := s.PutProfile(t.Context(), domain.NewProfile("u", nil, map[string]string{"score": "99"}))
	if err == nil {
		t.Fatal("cache invalidation failure reported as success")
	}
	stored, err := source.FindProfile(t.Context(), "u")
	if err != nil || stored.Fields["score"] != "99" {
		t.Fatal("persistent write lost", err)
	}
}
