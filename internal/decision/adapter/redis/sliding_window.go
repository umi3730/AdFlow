package redisadapter

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

var slidingWindowScript = redis.NewScript(`
local current_time = redis.call('TIME')
local now_ms = current_time[1] * 1000 + math.floor(current_time[2] / 1000)
local window_ms = tonumber(ARGV[1])
local limit = tonumber(ARGV[2])
local cutoff = now_ms - window_ms

redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', cutoff)
local count = redis.call('ZCARD', KEYS[1])
if count >= limit then
    redis.call('PEXPIRE', KEYS[1], tonumber(ARGV[4]))
    return {0, count}
end

redis.call('ZADD', KEYS[1], now_ms, ARGV[3])
redis.call('PEXPIRE', KEYS[1], tonumber(ARGV[4]))
return {1, count + 1}
`)

type SlidingWindowLimiter struct {
	client     *redis.Client
	key        string
	limit      int64
	window     time.Duration
	ttl        time.Duration
	instanceID string
	sequence   atomic.Uint64
}

func NewSlidingWindowLimiter(client *redis.Client, keyPrefix string, limit int, window time.Duration) (*SlidingWindowLimiter, error) {
	if client == nil || strings.TrimSpace(keyPrefix) == "" || limit <= 0 || window < time.Millisecond {
		return nil, errors.New("Redis sliding window requires a client, key prefix, positive limit, and millisecond window")
	}
	instanceID, err := randomLimiterID()
	if err != nil {
		return nil, fmt.Errorf("create limiter instance id: %w", err)
	}
	return &SlidingWindowLimiter{
		client: client, key: strings.TrimSuffix(keyPrefix, ":") + ":global", limit: int64(limit),
		window: window, ttl: 2 * window, instanceID: instanceID,
	}, nil
}

func (l *SlidingWindowLimiter) Allow(ctx context.Context, requestID string) (bool, error) {
	member := l.instanceID + ":" + strconv.FormatUint(l.sequence.Add(1), 10) + ":" + requestID
	result, err := slidingWindowScript.Run(ctx, l.client, []string{l.key},
		l.window.Milliseconds(), l.limit, member, l.ttl.Milliseconds()).Slice()
	if err != nil {
		return false, fmt.Errorf("run Redis sliding-window limiter: %w", err)
	}
	if len(result) != 2 {
		return false, errors.New("Redis sliding-window limiter returned an invalid result")
	}
	allowed, ok := result[0].(int64)
	if !ok {
		return false, errors.New("Redis sliding-window limiter returned an invalid allowance")
	}
	return allowed == 1, nil
}

func randomLimiterID() (string, error) {
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}
