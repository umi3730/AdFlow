package redisadapter

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type Reservations struct {
	client *redis.Client
	prefix string
}

func NewReservations(client *redis.Client, prefix string) *Reservations {
	return &Reservations{client: client, prefix: strings.TrimSuffix(prefix, ":")}
}

var reserveFrequencyScript = redis.NewScript(`
local existing = redis.call('GET', KEYS[2])
if existing then return 1 end
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', ARGV[1])
if redis.call('ZCARD', KEYS[1]) >= tonumber(ARGV[2]) then return 0 end
redis.call('ZADD', KEYS[1], ARGV[3], ARGV[4])
redis.call('PEXPIREAT', KEYS[1], ARGV[5])
redis.call('SET', KEYS[2], ARGV[6], 'PX', ARGV[7])
return 1
`)

var releaseFrequencyScript = redis.NewScript(`
redis.call('ZREM', KEYS[1], ARGV[1])
redis.call('DEL', KEYS[2])
return 1
`)

var confirmFrequencyScript = redis.NewScript(`
redis.call('ZADD', KEYS[1], ARGV[1], ARGV[2])
redis.call('PEXPIREAT', KEYS[1], ARGV[3])
redis.call('DEL', KEYS[2])
return 1
`)

func (r *Reservations) ReserveFrequency(ctx context.Context, userID, campaignID, requestID string, limit uint32, now time.Time, ttl time.Duration) (string, bool, error) {
	base := fmt.Sprintf("%s:%s:%s", now.UTC().Format("2006-01-02"), campaignID, userID)
	frequencyKey := r.prefix + ":freq:" + base
	metaKey := r.prefix + ":freq:rsv:" + requestID
	dayEnd := endOfDay(now)
	value, err := reserveFrequencyScript.Run(ctx, r.client, []string{frequencyKey, metaKey},
		now.UnixMilli(), limit, now.Add(ttl).UnixMilli(), requestID, dayEnd.Add(24*time.Hour).UnixMilli(), base, ttl.Milliseconds(),
	).Int()
	return requestID, value == 1, err
}

func (r *Reservations) ReleaseFrequency(ctx context.Context, token string) error {
	metaKey := r.prefix + ":freq:rsv:" + token
	base, err := r.client.Get(ctx, metaKey).Result()
	if err == redis.Nil {
		return nil
	}
	if err != nil {
		return err
	}
	return releaseFrequencyScript.Run(ctx, r.client, []string{r.prefix + ":freq:" + base, metaKey}, token).Err()
}

func (r *Reservations) ConfirmFrequency(ctx context.Context, token string, now time.Time) error {
	metaKey := r.prefix + ":freq:rsv:" + token
	base, err := r.client.Get(ctx, metaKey).Result()
	if err == redis.Nil {
		return nil
	}
	if err != nil {
		return err
	}
	dayEnd := endOfDay(now)
	return confirmFrequencyScript.Run(ctx, r.client, []string{r.prefix + ":freq:" + base, metaKey}, dayEnd.UnixMilli(), token, dayEnd.Add(24*time.Hour).UnixMilli()).Err()
}

var reserveBudgetScript = redis.NewScript(`
local expired = redis.call('ZRANGEBYSCORE', KEYS[4], '-inf', ARGV[1])
for _, token in ipairs(expired) do
  local amount = tonumber(redis.call('HGET', KEYS[3], token) or '0')
  if amount > 0 then redis.call('DECRBY', KEYS[2], amount) end
  redis.call('HDEL', KEYS[3], token)
  redis.call('ZREM', KEYS[4], token)
end
if redis.call('HEXISTS', KEYS[3], ARGV[4]) == 1 then return 1 end
local spent = tonumber(redis.call('GET', KEYS[1]) or '0')
local reserved = tonumber(redis.call('GET', KEYS[2]) or '0')
local budget = tonumber(ARGV[2])
local cost = tonumber(ARGV[3])
if cost <= 0 or spent + reserved + cost > budget then return 0 end
redis.call('INCRBY', KEYS[2], cost)
redis.call('HSET', KEYS[3], ARGV[4], cost)
redis.call('ZADD', KEYS[4], ARGV[5], ARGV[4])
redis.call('SET', KEYS[5], ARGV[6], 'PX', ARGV[7])
for i = 1, 4 do redis.call('PEXPIREAT', KEYS[i], ARGV[8]) end
return 1
`)

var settleBudgetScript = redis.NewScript(`
local amount = tonumber(redis.call('HGET', KEYS[3], ARGV[1]) or '0')
if amount == 0 then redis.call('DEL', KEYS[5]); return 0 end
redis.call('DECRBY', KEYS[2], amount)
if ARGV[2] == 'confirm' then redis.call('INCRBY', KEYS[1], amount) end
redis.call('HDEL', KEYS[3], ARGV[1])
redis.call('ZREM', KEYS[4], ARGV[1])
redis.call('DEL', KEYS[5])
return 1
`)

func (r *Reservations) ReserveBudget(ctx context.Context, campaignID string, dailyBudgetFen, costFen int64, requestID string, now time.Time, ttl time.Duration) (string, bool, error) {
	base := fmt.Sprintf("%s:%s", now.UTC().Format("2006-01-02"), campaignID)
	keys := r.budgetKeys(base, requestID)
	value, err := reserveBudgetScript.Run(ctx, r.client, keys,
		now.UnixMilli(), dailyBudgetFen, costFen, requestID, now.Add(ttl).UnixMilli(), base, ttl.Milliseconds(), endOfDay(now).Add(24*time.Hour).UnixMilli(),
	).Int()
	return requestID, value == 1, err
}

func (r *Reservations) ReleaseBudget(ctx context.Context, token string) error {
	return r.settleBudget(ctx, token, "release")
}

func (r *Reservations) ConfirmBudget(ctx context.Context, token string, _ time.Time) error {
	return r.settleBudget(ctx, token, "confirm")
}

func (r *Reservations) settleBudget(ctx context.Context, token, action string) error {
	metaKey := r.prefix + ":budget:rsv:" + token
	base, err := r.client.Get(ctx, metaKey).Result()
	if err == redis.Nil {
		return nil
	}
	if err != nil {
		return err
	}
	return settleBudgetScript.Run(ctx, r.client, r.budgetKeys(base, token), token, action).Err()
}

func (r *Reservations) budgetKeys(base, token string) []string {
	root := r.prefix + ":budget:" + base
	return []string{root + ":spent", root + ":reserved", root + ":amounts", root + ":expires", r.prefix + ":budget:rsv:" + token}
}

func endOfDay(now time.Time) time.Time {
	utc := now.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day()+1, 0, 0, 0, 0, time.UTC)
}

func (r *Reservations) DebugBudgetSpent(ctx context.Context, campaignID string, now time.Time) (int64, error) {
	key := r.prefix + ":budget:" + now.UTC().Format("2006-01-02") + ":" + campaignID + ":spent"
	value, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(value, 10, 64)
}
