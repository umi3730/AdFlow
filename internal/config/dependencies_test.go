package config

import "testing"

func TestDependencyRequirementsFollowSelectedAdapters(t *testing.T) {
	for _, tc := range []struct {
		name         string
		cfg          Config
		mysql, redis bool
	}{
		{"memory demo", Config{CampaignRepository: "memory", DecisionStore: "memory", ProfileStore: "memory", AuthStore: "memory", AuditStore: "memory", ReservationAdapter: "memory", DecisionRateLimiter: "memory", EventTransport: "sync"}, false, false},
		{"unused connection settings", Config{MySQLDSN: "invalid-unused-dsn", RedisAddr: "127.0.0.1:1"}, false, false},
		{"campaign persistence", Config{CampaignRepository: "mysql"}, true, false},
		{"decision persistence", Config{DecisionStore: "mysql"}, true, false},
		{"profile persistence", Config{ProfileStore: "mysql"}, true, false},
		{"cached profiles", Config{ProfileStore: "mysql-redis"}, true, true},
		{"auth persistence", Config{AuthStore: "mysql"}, true, false},
		{"audit persistence", Config{AuditStore: "mysql"}, true, false},
		{"reservations", Config{ReservationAdapter: "redis"}, false, true},
		{"shared limiter", Config{DecisionRateLimiter: "redis"}, false, true},
		{"kafka settlement", Config{EventTransport: "kafka"}, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.cfg.RequiresMySQL() != tc.mysql || tc.cfg.RequiresRedis() != tc.redis {
				t.Fatalf("mysql=%v redis=%v", tc.cfg.RequiresMySQL(), tc.cfg.RequiresRedis())
			}
		})
	}
}
