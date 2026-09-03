package cache

import (
	"context"

	"github.com/redis/go-redis/v9"
	"github.com/redis/go-redis/v9/maintnotifications"
)

type Redis struct {
	client *redis.Client
}

func NewRedis(addr, password string, db int) *Redis {
	return &Redis{client: redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           db,
		PoolSize:     30,
		MinIdleConns: 5,
		MaintNotificationsConfig: &maintnotifications.Config{
			Mode: maintnotifications.ModeDisabled,
		},
	})}
}

func (r *Redis) PingContext(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

func (r *Redis) Close() error { return r.client.Close() }

func (r *Redis) Client() *redis.Client { return r.client }
