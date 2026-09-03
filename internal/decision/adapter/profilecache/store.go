package profilecache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/zhanghaiyang/adflow/internal/decision/domain"
)

type Observer interface {
	ObserveProfileCache(string, time.Duration)
}

type profilePayload struct {
	Missing bool              `json:"missing,omitempty"`
	Tags    []string          `json:"tags,omitempty"`
	Fields  map[string]string `json:"fields,omitempty"`
}

type profileLoad struct {
	done    chan struct{}
	profile domain.Profile
	err     error
}

type Store struct {
	source      domain.ProfileStore
	client      *redis.Client
	prefix      string
	ttl         time.Duration
	negativeTTL time.Duration
	timeout     time.Duration
	observer    Observer

	mu       sync.Mutex
	inFlight map[string]*profileLoad
	keyLocks [256]sync.Mutex
}

func New(source domain.ProfileStore, client *redis.Client, prefix string, ttl, negativeTTL, timeout time.Duration, observer Observer) (*Store, error) {
	if source == nil || client == nil || strings.TrimSpace(prefix) == "" || ttl <= 0 || negativeTTL <= 0 || negativeTTL > ttl || timeout <= 0 {
		return nil, errors.New("profile cache requires a source, Redis client, prefix, and valid TTLs/timeouts")
	}
	return &Store{
		source: source, client: client, prefix: strings.TrimSuffix(prefix, ":"), ttl: ttl,
		negativeTTL: negativeTTL, timeout: timeout, observer: observer, inFlight: make(map[string]*profileLoad),
	}, nil
}

func (s *Store) FindProfile(ctx context.Context, userID string) (domain.Profile, error) {
	started := time.Now()
	payload, err := s.get(ctx, s.key(userID))
	switch {
	case err == nil:
		profile, decodeErr := decodeProfile(userID, payload)
		if decodeErr == nil {
			if profile.Missing {
				s.observe("negative_hit", time.Since(started))
				return domain.Profile{}, domain.ErrProfileNotFound
			}
			s.observe("hit", time.Since(started))
			return domain.NewProfile(userID, profile.Tags, profile.Fields), nil
		}
		_ = s.del(ctx, s.key(userID))
		s.observe("decode_error", time.Since(started))
	case errors.Is(err, redis.Nil):
		s.observe("miss", time.Since(started))
	default:
		// MySQL remains authoritative. A cache outage degrades latency instead
		// of turning profile lookup into a decision outage.
		s.observe("error", time.Since(started))
	}
	return s.loadSource(ctx, userID)
}

func (s *Store) PutProfile(ctx context.Context, profile domain.Profile) error {
	keyLock := s.keyLock(profile.UserID)
	keyLock.Lock()
	defer keyLock.Unlock()
	if err := s.source.PutProfile(ctx, profile); err != nil {
		return err
	}
	payload, err := encodeProfile(profile)
	if err != nil {
		return err
	}
	started := time.Now()
	if err := s.set(ctx, s.key(profile.UserID), payload, s.ttl); err != nil {
		s.observe("write_error", time.Since(started))
		return fmt.Errorf("update profile cache after persistent write: %w", err)
	}
	s.observe("write", time.Since(started))
	return nil
}

func (s *Store) loadSource(ctx context.Context, userID string) (domain.Profile, error) {
	s.mu.Lock()
	if load, exists := s.inFlight[userID]; exists {
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return domain.Profile{}, ctx.Err()
		case <-load.done:
			s.observe("shared", 0)
			return cloneProfile(load.profile), load.err
		}
	}
	load := &profileLoad{done: make(chan struct{})}
	s.inFlight[userID] = load
	s.mu.Unlock()

	keyLock := s.keyLock(userID)
	keyLock.Lock()
	started := time.Now()
	profile, err := s.source.FindProfile(ctx, userID)
	cacheResult := "fill"
	if errors.Is(err, domain.ErrProfileNotFound) {
		cacheResult = "negative_fill"
		payload, _ := json.Marshal(profilePayload{Missing: true})
		if setErr := s.set(ctx, s.key(userID), payload, s.negativeTTL); setErr != nil {
			cacheResult = "negative_fill_error"
		}
	} else if err == nil {
		if payload, encodeErr := encodeProfile(profile); encodeErr == nil {
			if setErr := s.set(ctx, s.key(userID), payload, s.ttl); setErr != nil {
				cacheResult = "fill_error"
			}
		} else {
			cacheResult = "fill_error"
		}
	}
	keyLock.Unlock()
	s.mu.Lock()
	load.profile = cloneProfile(profile)
	load.err = err
	delete(s.inFlight, userID)
	close(load.done)
	s.mu.Unlock()
	if err == nil {
		s.observe(cacheResult, time.Since(started))
	} else if errors.Is(err, domain.ErrProfileNotFound) {
		s.observe(cacheResult, time.Since(started))
	} else {
		s.observe("source_error", time.Since(started))
	}
	return cloneProfile(profile), err
}

func (s *Store) key(userID string) string { return s.prefix + ":" + userID }

func (s *Store) keyLock(userID string) *sync.Mutex {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(userID))
	return &s.keyLocks[hash.Sum32()%uint32(len(s.keyLocks))]
}

func (s *Store) get(ctx context.Context, key string) ([]byte, error) {
	cacheCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.client.Get(cacheCtx, key).Bytes()
}

func (s *Store) set(ctx context.Context, key string, payload []byte, ttl time.Duration) error {
	cacheCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.client.Set(cacheCtx, key, payload, ttl).Err()
}

func (s *Store) del(ctx context.Context, key string) error {
	cacheCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.client.Del(cacheCtx, key).Err()
}

func (s *Store) observe(result string, duration time.Duration) {
	if s.observer != nil {
		s.observer.ObserveProfileCache(result, duration)
	}
}

func encodeProfile(profile domain.Profile) ([]byte, error) {
	tags := make([]string, 0, len(profile.Tags))
	for tag := range profile.Tags {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return json.Marshal(profilePayload{Tags: tags, Fields: profile.Fields})
}

func decodeProfile(userID string, payload []byte) (profilePayload, error) {
	var profile profilePayload
	if err := json.Unmarshal(payload, &profile); err != nil {
		return profilePayload{}, fmt.Errorf("decode cached profile %s: %w", userID, err)
	}
	return profile, nil
}

func cloneProfile(profile domain.Profile) domain.Profile {
	tags := make([]string, 0, len(profile.Tags))
	for tag := range profile.Tags {
		tags = append(tags, tag)
	}
	return domain.NewProfile(profile.UserID, tags, profile.Fields)
}
