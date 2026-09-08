package profilecache

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"github.com/redis/go-redis/v9"
	"time"
)

var readGeneration = redis.NewScript(`
local value = redis.call('GET', KEYS[1])
if value then return value end
redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
return ARGV[1]
`)
var invalidateGeneration = redis.NewScript(`
redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
redis.call('DEL', KEYS[2])
return 1
`)
var fillGeneration = redis.NewScript(`
if redis.call('GET', KEYS[1]) ~= ARGV[1] then return 0 end
redis.call('SET', KEYS[2], ARGV[2], 'PX', ARGV[3])
redis.call('PEXPIRE', KEYS[1], ARGV[4])
return 1
`)

func generationToken() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}
func (s *Store) generationKey(userID string) string { return s.prefix + ":generation:" + userID }
func (s *Store) generationTTL() time.Duration       { return s.ttl + s.negativeTTL + time.Minute }

func (s *Store) generation(ctx context.Context, userID string) (string, error) {
	token, err := generationToken()
	if err != nil {
		return "", err
	}
	cacheCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return readGeneration.Run(cacheCtx, s.client, []string{s.generationKey(userID)}, token, s.generationTTL().Milliseconds()).Text()
}

// Every completed mutation invalidates, rather than writing its input into
// cache. Out-of-order writers therefore cannot cache an older input value.
func (s *Store) invalidate(ctx context.Context, userID string) error {
	token, err := generationToken()
	if err != nil {
		return err
	}
	cacheCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.timeout)
	defer cancel()
	return invalidateGeneration.Run(cacheCtx, s.client, []string{s.generationKey(userID), s.key(userID)}, token, s.generationTTL().Milliseconds()).Err()
}

func (s *Store) fill(ctx context.Context, userID, generation string, payload []byte, ttl time.Duration) (bool, error) {
	if generation == "" {
		return false, nil
	}
	cacheCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	value, err := fillGeneration.Run(cacheCtx, s.client, []string{s.generationKey(userID), s.key(userID)}, generation, payload, ttl.Milliseconds(), s.generationTTL().Milliseconds()).Int()
	return value == 1, err
}
