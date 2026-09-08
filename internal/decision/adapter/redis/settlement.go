package redisadapter

import (
	"context"
	"github.com/redis/go-redis/v9"
	"github.com/zhanghaiyang/adflow/internal/decision/domain"
	"strings"
	"time"
)

// Both gates and the retry receipt are changed in one Redis operation. Missing
// metadata without a matching receipt is ambiguous, never successful settlement.
var settleImpressionScript = redis.NewScript(`
local receipt = redis.call('GET', KEYS[8])
if receipt then
 if receipt == ARGV[2] then return 1 else return 0 end
end
if redis.call('GET', KEYS[5]) ~= ARGV[3] or redis.call('GET', KEYS[7]) ~= ARGV[4] then return 0 end
local be = redis.call('ZSCORE', KEYS[4], ARGV[1])
local fe = redis.call('ZSCORE', KEYS[6], ARGV[1])
local budgetTTL = redis.call('PTTL', KEYS[2])
local frequencyTTL = redis.call('PTTL', KEYS[6])
local amount = tonumber(redis.call('HGET', KEYS[3], ARGV[1]) or '0')
if not be or not fe or redis.call('PTTL', KEYS[5]) <= 0 or redis.call('PTTL', KEYS[7]) <= 0 then return 0 end
if budgetTTL <= 0 or frequencyTTL <= 0 or amount <= 0 or amount ~= tonumber(ARGV[5]) then return 0 end
if tonumber(redis.call('GET', KEYS[2]) or '0') < amount then return 0 end
redis.call('DECRBY', KEYS[2], amount)
redis.call('INCRBY', KEYS[1], amount)
redis.call('PEXPIRE', KEYS[1], budgetTTL)
redis.call('HDEL', KEYS[3], ARGV[1])
redis.call('ZREM', KEYS[4], ARGV[1])
redis.call('ZADD', KEYS[6], ARGV[6], ARGV[1])
redis.call('PEXPIRE', KEYS[6], frequencyTTL)
redis.call('DEL', KEYS[5], KEYS[7])
redis.call('SET', KEYS[8], ARGV[2], 'PX', ARGV[7])
return 1
`)

func (r *Reservations) SettleImpression(ctx context.Context, settlement domain.Settlement) error {
	d := settlement.Decision
	if settlement.EventID == "" || d.ReservationToken == "" || d.Pricing.PriceFen <= 0 {
		return domain.ErrSettlementUnavailable
	}
	token, fingerprint := d.ReservationToken, settlement.Fingerprint()
	receiptKey := r.prefix + ":settled:" + token
	budgetMeta, frequencyMeta := r.prefix+":budget:rsv:"+token, r.prefix+":freq:rsv:"+token
	values, err := r.client.MGet(ctx, receiptKey, budgetMeta, frequencyMeta).Result()
	if err != nil {
		return err
	}
	if values[0] != nil {
		if values[0] == fingerprint {
			return nil
		}
		return domain.ErrSettlementUnavailable
	}
	budgetBase, bok := values[1].(string)
	frequencyBase, fok := values[2].(string)
	if !bok || !fok {
		// Another worker may have confirmed between MGET and this check.
		receipt, err := r.client.Get(ctx, receiptKey).Result()
		if err == nil && receipt == fingerprint {
			return nil
		}
		if err != nil && err != redis.Nil {
			return err
		}
		return domain.ErrSettlementUnavailable
	}
	if len(budgetBase) < 11 || !strings.HasSuffix(budgetBase, ":"+d.CampaignID) {
		return domain.ErrSettlementUnavailable
	}
	day, err := time.Parse("2006-01-02", budgetBase[:10])
	if err != nil || budgetBase != day.Format("2006-01-02")+":"+d.CampaignID || frequencyBase != budgetBase+":"+d.UserID {
		return domain.ErrSettlementUnavailable
	}
	keys := append(r.budgetKeys(budgetBase, token), r.prefix+":freq:"+frequencyBase, frequencyMeta, receiptKey)
	value, err := settleImpressionScript.Run(ctx, r.client, keys, token, fingerprint, budgetBase, frequencyBase,
		d.Pricing.PriceFen, day.Add(24*time.Hour).UnixMilli(), (7 * 24 * time.Hour).Milliseconds()).Int()
	if err != nil {
		return err
	}
	if value != 1 {
		return domain.ErrSettlementUnavailable
	}
	return nil
}
