package database

import "testing"

func TestPoolConfigurationRejectsUnboundedOrInconsistentLimits(t *testing.T) {
	for _, cfg := range []PoolConfig{{}, {MaxOpenConns: 30, MaxIdleConns: 31}, {MaxOpenConns: 30, MaxIdleConns: -1}} {
		if db, err := OpenMySQLWithConfig("test@tcp(127.0.0.1:1)/unused", cfg); err == nil {
			db.Close()
			t.Fatal("invalid pool accepted")
		}
	}
	cfg := DefaultPoolConfig()
	if cfg.MaxOpenConns != 30 || cfg.MaxIdleConns != 10 {
		t.Fatalf("unexpected default: %+v", cfg)
	}
	for _, max := range []int{30, 60, 100} {
		cfg.MaxOpenConns = max
		db, err := OpenMySQLWithConfig("test@tcp(127.0.0.1:1)/unused", cfg)
		if err != nil {
			t.Fatal(err)
		}
		if db.Stats().MaxOpenConnections != max {
			t.Fatal("limit not applied")
		}
		db.Close()
	}
}
