package profilecache

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/zhanghaiyang/adflow/internal/decision/domain"
)

type fakeProfiles struct {
	mu       sync.Mutex
	profiles map[string]domain.Profile
	finds    atomic.Int64
}

type blockingProfiles struct {
	profile domain.Profile
	finds   atomic.Int64
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *blockingProfiles) FindProfile(context.Context, string) (domain.Profile, error) {
	b.finds.Add(1)
	b.once.Do(func() { close(b.started) })
	<-b.release
	return cloneProfile(b.profile), nil
}

func (b *blockingProfiles) PutProfile(context.Context, domain.Profile) error { return nil }

func (f *fakeProfiles) FindProfile(_ context.Context, userID string) (domain.Profile, error) {
	f.finds.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	profile, exists := f.profiles[userID]
	if !exists {
		return domain.Profile{}, domain.ErrProfileNotFound
	}
	return cloneProfile(profile), nil
}

func (f *fakeProfiles) PutProfile(_ context.Context, profile domain.Profile) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.profiles[profile.UserID] = cloneProfile(profile)
	return nil
}

func testStore(t *testing.T, source domain.ProfileStore) (*Store, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), Protocol: 2, DisableIdentity: true})
	t.Cleanup(func() { _ = client.Close() })
	store, err := New(source, client, "adflow:test:profile", time.Minute, 5*time.Second, 20*time.Millisecond, nil)
	if err != nil {
		t.Fatal(err)
	}
	return store, server
}

func TestProfileCacheAsideFillsAndReturnsDefensiveCopies(t *testing.T) {
	source := &fakeProfiles{profiles: map[string]domain.Profile{
		"user-1": domain.NewProfile("user-1", []string{"anime"}, map[string]string{"device": "ios"}),
	}}
	store, _ := testStore(t, source)
	first, err := store.FindProfile(context.Background(), "user-1")
	if err != nil {
		t.Fatal(err)
	}
	first.Tags["mutated"] = struct{}{}
	first.Fields["device"] = "mutated"
	second, err := store.FindProfile(context.Background(), "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if source.finds.Load() != 1 || second.Fields["device"] != "ios" {
		t.Fatalf("finds=%d profile=%+v", source.finds.Load(), second)
	}
	if _, exists := second.Tags["mutated"]; exists {
		t.Fatal("cached profile was mutated through a returned map")
	}
}

func TestProfileCacheAsideNegativeCachesMissingUsers(t *testing.T) {
	source := &fakeProfiles{profiles: make(map[string]domain.Profile)}
	store, server := testStore(t, source)
	for range 2 {
		if _, err := store.FindProfile(context.Background(), "missing"); err != domain.ErrProfileNotFound {
			t.Fatalf("error=%v", err)
		}
	}
	if source.finds.Load() != 1 {
		t.Fatalf("finds=%d", source.finds.Load())
	}
	server.FastForward(6 * time.Second)
	if _, err := store.FindProfile(context.Background(), "missing"); err != domain.ErrProfileNotFound {
		t.Fatalf("error after expiry=%v", err)
	}
	if source.finds.Load() != 2 {
		t.Fatalf("finds after expiry=%d", source.finds.Load())
	}
}

func TestProfileCacheAsideWriteThroughReplacesCachedProfile(t *testing.T) {
	source := &fakeProfiles{profiles: map[string]domain.Profile{
		"user-1": domain.NewProfile("user-1", []string{"old"}, nil),
	}}
	store, _ := testStore(t, source)
	if _, err := store.FindProfile(context.Background(), "user-1"); err != nil {
		t.Fatal(err)
	}
	updated := domain.NewProfile("user-1", []string{"new"}, map[string]string{"device": "android"})
	if err := store.PutProfile(context.Background(), updated); err != nil {
		t.Fatal(err)
	}
	profile, err := store.FindProfile(context.Background(), "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := profile.Tags["new"]; !exists || profile.Fields["device"] != "android" || source.finds.Load() != 1 {
		t.Fatalf("finds=%d profile=%+v", source.finds.Load(), profile)
	}
}

func TestProfileCacheAsideFallsBackWhenRedisIsUnavailable(t *testing.T) {
	source := &fakeProfiles{profiles: map[string]domain.Profile{
		"user-1": domain.NewProfile("user-1", []string{"anime"}, nil),
	}}
	store, server := testStore(t, source)
	server.Close()
	started := time.Now()
	profile, err := store.FindProfile(context.Background(), "user-1")
	if err != nil || profile.UserID != "user-1" {
		t.Fatalf("profile=%+v err=%v", profile, err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("Redis fallback took %s", elapsed)
	}
}

func TestProfileCacheAsideCoalescesConcurrentMisses(t *testing.T) {
	source := &blockingProfiles{
		profile: domain.NewProfile("user-1", []string{"anime"}, nil),
		started: make(chan struct{}), release: make(chan struct{}),
	}
	store, _ := testStore(t, source)
	const workers = 32
	start := make(chan struct{})
	errors := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, err := store.FindProfile(context.Background(), "user-1")
			errors <- err
		}()
	}
	close(start)
	<-source.started
	close(source.release)
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	if source.finds.Load() != 1 {
		t.Fatalf("source finds=%d", source.finds.Load())
	}
}
