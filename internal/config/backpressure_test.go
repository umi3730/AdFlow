package config

import "testing"

func TestBackpressureConfiguration(t *testing.T) {
	cfg, err := Load()
	if err != nil || !cfg.AsyncBackpressure || cfg.SettlementBacklogHigh != 100 || cfg.SettlementBacklogLow != 40 || cfg.OutboxBacklogHigh != 600 || cfg.OutboxBacklogLow != 240 {
		t.Fatalf("defaults: %v", err)
	}
	t.Setenv("ADFLOW_ASYNC_BACKPRESSURE", "false")
	t.Setenv("ADFLOW_SETTLEMENT_BACKLOG_HIGH", "80")
	t.Setenv("ADFLOW_SETTLEMENT_BACKLOG_LOW", "20")
	cfg, err = Load()
	if err != nil || cfg.AsyncBackpressure || cfg.SettlementBacklogHigh != 80 || cfg.SettlementBacklogLow != 20 {
		t.Fatalf("overrides: %v", err)
	}
	for _, pair := range [][2]string{{"ADFLOW_SETTLEMENT_BACKLOG_LOW", "80"}, {"ADFLOW_OUTBOX_BACKLOG_HIGH", "0"}, {"ADFLOW_OUTBOX_BACKLOG_LOW", "600"}} {
		t.Run(pair[0], func(t *testing.T) {
			t.Setenv(pair[0], pair[1])
			if _, err := Load(); err == nil {
				t.Fatal("invalid thresholds accepted")
			}
		})
	}
}
