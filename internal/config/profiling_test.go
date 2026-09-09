package config

import "testing"

func TestProfilingAndPoolConfiguration(t *testing.T) {
	t.Setenv("ADFLOW_PPROF_ADDR", "127.0.0.1:6061")
	t.Setenv("ADFLOW_MYSQL_MAX_OPEN_CONNS", "60")
	t.Setenv("ADFLOW_MYSQL_MAX_IDLE_CONNS", "10")
	cfg, err := Load()
	if err != nil || cfg.PprofAddr != "127.0.0.1:6061" || cfg.MySQLMaxOpenConns != 60 {
		t.Fatal("configuration not applied", err)
	}
	t.Setenv("ADFLOW_MYSQL_MAX_IDLE_CONNS", "61")
	if _, err := Load(); err == nil {
		t.Fatal("invalid idle count accepted")
	}
	t.Setenv("ADFLOW_MYSQL_MAX_IDLE_CONNS", "10")
	t.Setenv("ADFLOW_PPROF_ADDR", ":6060")
	if _, err := Load(); err == nil {
		t.Fatal("public profiler address accepted")
	}
}
