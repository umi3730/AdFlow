package redisadapter

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestSlidingWindowExpiresOldRequests(t *testing.T) {
	server := miniredis.RunT(t)
	baseTime := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	server.SetTime(baseTime)
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), Protocol: 2, DisableIdentity: true})
	defer client.Close()
	limiter, err := NewSlidingWindowLimiter(client, "adflow:test", 2, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 2; index++ {
		allowed, err := limiter.Allow(t.Context(), "request")
		if err != nil || !allowed {
			t.Fatalf("request %d allowed=%v err=%v", index, allowed, err)
		}
	}
	allowed, err := limiter.Allow(t.Context(), "rejected")
	if err != nil || allowed {
		t.Fatalf("third request allowed=%v err=%v", allowed, err)
	}
	server.SetTime(baseTime.Add(time.Second + time.Millisecond))
	allowed, err = limiter.Allow(t.Context(), "after-window")
	if err != nil || !allowed {
		t.Fatalf("request after window allowed=%v err=%v", allowed, err)
	}
}

func TestSlidingWindowIsAtomicUnderConcurrency(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), Protocol: 2, DisableIdentity: true})
	defer client.Close()
	limiter, err := NewSlidingWindowLimiter(client, "adflow:concurrent", 10, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var allowedCount atomic.Int64
	var wait sync.WaitGroup
	for index := 0; index < 50; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			allowed, callErr := limiter.Allow(context.Background(), "same-request-id")
			if callErr != nil {
				t.Errorf("Allow() error=%v", callErr)
				return
			}
			if allowed {
				allowedCount.Add(1)
			}
		}()
	}
	wait.Wait()
	if allowedCount.Load() != 10 {
		t.Fatalf("allowed=%d, want 10", allowedCount.Load())
	}
	if ttl := server.TTL("adflow:concurrent:global"); ttl <= 0 {
		t.Fatalf("expected expiring limiter key, ttl=%s", ttl)
	}
}
