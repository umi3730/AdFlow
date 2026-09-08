//go:build integration

package integration

import (
	"context"
	"errors"
	"github.com/redis/go-redis/v9"
	profilemysql "github.com/zhanghaiyang/adflow/internal/decision/adapter/mysql"
	profilecache "github.com/zhanghaiyang/adflow/internal/decision/adapter/profilecache"
	"github.com/zhanghaiyang/adflow/internal/decision/domain"
	"sync"
	"testing"
	"time"
)

type pausedProfileRead struct {
	domain.ProfileStore
	captured, resume chan struct{}
	once             sync.Once
}

func (s *pausedProfileRead) FindProfile(ctx context.Context, id string) (domain.Profile, error) {
	p, err := s.ProfileStore.FindProfile(ctx, id)
	s.once.Do(func() {
		close(s.captured)
		select {
		case <-s.resume:
		case <-ctx.Done():
		}
	})
	return p, err
}

func TestProfileGenerationRejectsCrossInstanceStaleRefill(t *testing.T) {
	firstDB, secondDB := integrationMySQL(t), integrationMySQL(t)
	firstSource, secondSource := profilemysql.NewProfileStore(firstDB), profilemysql.NewProfileStore(secondDB)
	client := redis.NewClient(&redis.Options{Addr: integrationEnv(t, "ADFLOW_IT_REDIS_ADDR"), Protocol: 2, DisableIdentity: true})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	prefix := "adflow:it:generation:" + newID(t)
	t.Cleanup(func() {
		if err := deletePrefix(context.Background(), client, prefix); err != nil {
			t.Errorf("clean Redis test keys: %v", err)
		}
	})
	for _, operation := range []string{"update", "delete", "recreate"} {
		t.Run(operation, func(t *testing.T) {
			id := "generation-test-" + newID(t)
			t.Cleanup(func() { _ = firstSource.DeleteProfile(context.Background(), id) })
			if err := firstSource.PutProfile(t.Context(), domain.NewProfile(id, nil, map[string]string{"score": "10"})); err != nil {
				t.Fatal(err)
			}
			paused := &pausedProfileRead{ProfileStore: firstSource, captured: make(chan struct{}), resume: make(chan struct{})}
			reader, err := profilecache.New(paused, client, prefix, time.Minute, time.Second, time.Second, nil)
			if err != nil {
				t.Fatal(err)
			}
			writer, err := profilecache.New(secondSource, client, prefix, time.Minute, time.Second, time.Second, nil)
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan struct{})
			go func() { defer close(done); _, _ = reader.FindProfile(t.Context(), id) }()
			<-paused.captured
			if operation != "update" {
				if err := writer.DeleteProfile(t.Context(), id); err != nil {
					t.Fatal(err)
				}
			}
			if operation != "delete" {
				if err := writer.PutProfile(t.Context(), domain.NewProfile(id, nil, map[string]string{"score": "99"})); err != nil {
					t.Fatal(err)
				}
			}
			close(paused.resume)
			<-done
			for range 2 {
				got, err := writer.FindProfile(t.Context(), id)
				if operation == "delete" {
					if !errors.Is(err, domain.ErrProfileNotFound) {
						t.Fatalf("deleted profile cached: %v", err)
					}
				} else if err != nil || got.Fields["score"] != "99" {
					t.Fatalf("stale refill: %+v %v", got, err)
				}
			}
		})
	}
}
