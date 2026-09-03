package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	keys := []string{"ADFLOW_ENV", "ADFLOW_HTTP_ADDR", "ADFLOW_SHUTDOWN_TIMEOUT", "ADFLOW_DEPENDENCY_TIMEOUT", "ADFLOW_MYSQL_DSN", "ADFLOW_REDIS_ADDR", "ADFLOW_REDIS_PASSWORD", "ADFLOW_REDIS_DB"}
	for _, key := range keys {
		t.Setenv(key, "")
	}
	// Empty values are intentional values, so restore only the required defaults.
	t.Setenv("ADFLOW_HTTP_ADDR", ":8080")
	t.Setenv("ADFLOW_MYSQL_DSN", "dsn")
	t.Setenv("ADFLOW_REDIS_ADDR", "localhost:6379")
	t.Setenv("ADFLOW_SHUTDOWN_TIMEOUT", "10s")
	t.Setenv("ADFLOW_DEPENDENCY_TIMEOUT", "800ms")
	t.Setenv("ADFLOW_REDIS_DB", "0")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTPAddr != ":8080" || cfg.ShutdownTimeout != 10*time.Second || cfg.RedisDB != 0 {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadRejectsInvalidDuration(t *testing.T) {
	t.Setenv("ADFLOW_SHUTDOWN_TIMEOUT", "never")
	if _, err := Load(); err == nil {
		t.Fatal("Load() expected an error")
	}
}

func TestLoadRejectsInvalidEventTransport(t *testing.T) {
	t.Setenv("ADFLOW_EVENT_TRANSPORT", "redis-stream")
	if _, err := Load(); err == nil {
		t.Fatal("Load() expected an error")
	}
}

func TestLoadRejectsShortJWTSecret(t *testing.T) {
	t.Setenv("ADFLOW_JWT_SECRET", "short")
	if _, err := Load(); err == nil {
		t.Fatal("Load() expected an error")
	}
}

func TestLoadRequiresUsersWhenAuthEnabledOutsideLocal(t *testing.T) {
	t.Setenv("ADFLOW_ENV", "production")
	t.Setenv("ADFLOW_AUTH_ENABLED", "true")
	t.Setenv("ADFLOW_AUTH_USERS", "")
	if _, err := Load(); err == nil {
		t.Fatal("Load() expected an error")
	}
}

func TestLoadRejectsDevelopmentSecretOutsideLocal(t *testing.T) {
	t.Setenv("ADFLOW_ENV", "production")
	t.Setenv("ADFLOW_AUTH_ENABLED", "true")
	t.Setenv("ADFLOW_AUTH_USERS", "admin:admin:$2a$04$valid-looking-placeholder")
	if _, err := Load(); err == nil {
		t.Fatal("Load() expected an error")
	}
}

func TestLoadAuthenticationSettings(t *testing.T) {
	t.Setenv("ADFLOW_AUTH_ENABLED", "true")
	t.Setenv("ADFLOW_ACCESS_TOKEN_TTL", "45m")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.AuthEnabled || cfg.AccessTokenTTL != 45*time.Minute {
		t.Fatalf("unexpected authentication config: %+v", cfg)
	}
}

func TestLoadDecisionAdmissionSettings(t *testing.T) {
	t.Setenv("ADFLOW_CANDIDATE_CACHE_TTL", "7s")
	t.Setenv("ADFLOW_PROFILE_STORE", "mysql-redis")
	t.Setenv("ADFLOW_PROFILE_CACHE_TTL", "45s")
	t.Setenv("ADFLOW_PROFILE_NEGATIVE_CACHE_TTL", "3s")
	t.Setenv("ADFLOW_PROFILE_CACHE_TIMEOUT", "15ms")
	t.Setenv("ADFLOW_DECISION_RATE_LIMITER", "redis")
	t.Setenv("ADFLOW_DECISION_RATE_LIMIT", "2500.5")
	t.Setenv("ADFLOW_DECISION_RATE_WINDOW", "2s")
	t.Setenv("ADFLOW_DECISION_BURST", "300")
	t.Setenv("ADFLOW_DECISION_MAX_IN_FLIGHT", "64")
	t.Setenv("ADFLOW_DECISION_QUEUE_TIMEOUT", "7ms")
	t.Setenv("ADFLOW_DECISION_TIMEOUT", "90ms")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CandidateCacheTTL != 7*time.Second || cfg.ProfileStore != "mysql-redis" || cfg.ProfileCacheTTL != 45*time.Second || cfg.ProfileNegativeCacheTTL != 3*time.Second || cfg.ProfileCacheTimeout != 15*time.Millisecond || cfg.DecisionRateLimiter != "redis" || cfg.DecisionRateLimit != 2500.5 || cfg.DecisionRateWindow != 2*time.Second || cfg.DecisionBurst != 300 || cfg.DecisionMaxInFlight != 64 || cfg.DecisionQueueTimeout != 7*time.Millisecond || cfg.DecisionTimeout != 90*time.Millisecond {
		t.Fatalf("unexpected decision admission config: %+v", cfg)
	}
}

func TestLoadRejectsProfileNegativeTTLAbovePositiveTTL(t *testing.T) {
	t.Setenv("ADFLOW_PROFILE_CACHE_TTL", "5s")
	t.Setenv("ADFLOW_PROFILE_NEGATIVE_CACHE_TTL", "10s")
	if _, err := Load(); err == nil {
		t.Fatal("Load() expected an error")
	}
}

func TestLoadRejectsInvalidDecisionRateLimiter(t *testing.T) {
	t.Setenv("ADFLOW_DECISION_RATE_LIMITER", "database")
	if _, err := Load(); err == nil {
		t.Fatal("Load() expected an error")
	}
}

func TestLoadOpenAICompatibleAgentSettings(t *testing.T) {
	t.Setenv("ADFLOW_AGENT_PROVIDER", "openai-compatible")
	t.Setenv("ADFLOW_AGENT_API_KEY", "test-key")
	t.Setenv("ADFLOW_AGENT_MODEL", "test-model")
	t.Setenv("ADFLOW_AGENT_API_STYLE", "chat_completions")
	t.Setenv("ADFLOW_AGENT_THINKING", "disabled")
	t.Setenv("ADFLOW_AGENT_TIMEOUT", "5s")
	t.Setenv("ADFLOW_AGENT_MAX_RETRIES", "1")
	t.Setenv("ADFLOW_AGENT_FALLBACK_ENABLED", "false")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AgentProvider != "openai-compatible" || cfg.AgentModel != "test-model" || cfg.AgentAPIStyle != "chat_completions" || cfg.AgentThinkingMode != "disabled" || cfg.AgentTimeout != 5*time.Second || cfg.AgentMaxRetries != 1 || cfg.AgentFallbackEnabled {
		t.Fatalf("unexpected Agent config: %+v", cfg)
	}
}

func TestLoadOpenAICompatibleAgentRequiresCredentials(t *testing.T) {
	t.Setenv("ADFLOW_AGENT_PROVIDER", "openai-compatible")
	t.Setenv("ADFLOW_AGENT_API_KEY", "")
	t.Setenv("ADFLOW_AGENT_MODEL", "")
	if _, err := Load(); err == nil {
		t.Fatal("Load() expected an error")
	}
}

func TestLoadRejectsUnsafeAgentLimits(t *testing.T) {
	t.Setenv("ADFLOW_AGENT_MAX_RETRIES", "6")
	if _, err := Load(); err == nil {
		t.Fatal("Load() expected an error")
	}
	t.Setenv("ADFLOW_AGENT_MAX_RETRIES", "2")
	t.Setenv("ADFLOW_AGENT_MAX_DAILY_BUDGET_FEN", "100")
	t.Setenv("ADFLOW_AGENT_MAX_IMPRESSION_COST_FEN", "200")
	if _, err := Load(); err == nil {
		t.Fatal("Load() expected an error")
	}
}

func TestLoadRejectsInvalidDecisionAdmissionSettings(t *testing.T) {
	t.Setenv("ADFLOW_DECISION_MAX_IN_FLIGHT", "0")
	if _, err := Load(); err == nil {
		t.Fatal("Load() expected an error")
	}
	t.Setenv("ADFLOW_DECISION_MAX_IN_FLIGHT", "10")
	t.Setenv("ADFLOW_DECISION_QUEUE_TIMEOUT", "100ms")
	t.Setenv("ADFLOW_DECISION_TIMEOUT", "50ms")
	if _, err := Load(); err == nil {
		t.Fatal("Load() expected an error")
	}
}
