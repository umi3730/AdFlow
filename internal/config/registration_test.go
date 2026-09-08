package config

import (
	"os"
	"testing"
)

func TestRegistrationStoreDefaultsAndSwitch(t *testing.T) {
	t.Setenv("ADFLOW_ENV", "local")
	t.Setenv("ADFLOW_AUTH_STORE", "")
	t.Setenv("ADFLOW_REGISTRATION_ENABLED", "")
	_ = os.Unsetenv("ADFLOW_REGISTRATION_ENABLED")
	cfg, err := Load()
	if err != nil || cfg.RegistrationEnabled {
		t.Fatal(err)
	}
	t.Setenv("ADFLOW_CAMPAIGN_REPOSITORY", "mysql")
	cfg, err = Load()
	if err != nil || cfg.AuthStore != "mysql" {
		t.Fatalf("store=%s err=%v", cfg.AuthStore, err)
	}
	t.Setenv("ADFLOW_AUTH_STORE", "memory")
	t.Setenv("ADFLOW_REGISTRATION_ENABLED", "false")
	cfg, err = Load()
	if err != nil || cfg.RegistrationEnabled || cfg.AuthStore != "memory" {
		t.Fatal(err)
	}
	t.Setenv("ADFLOW_AUTH_STORE", "bad")
	if _, err = Load(); err == nil {
		t.Fatal("invalid auth store accepted")
	}
}
