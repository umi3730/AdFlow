package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

const localJWTSecret = "adflow-local-development-secret-change-me"

type Config struct {
	Environment               string
	HTTPAddr                  string
	ShutdownTimeout           time.Duration
	DependencyTimeout         time.Duration
	MySQLDSN                  string
	RedisAddr                 string
	RedisPassword             string
	RedisDB                   int
	CampaignRepository        string
	ReservationAdapter        string
	DecisionStore             string
	ProfileStore              string
	EventTransport            string
	KafkaBrokers              string
	KafkaTopic                string
	KafkaDeadLetterTopic      string
	KafkaConsumerGroup        string
	AuthEnabled               bool
	JWTSecret                 string
	JWTIssuer                 string
	AccessTokenTTL            time.Duration
	AuthUsers                 string
	AuditStore                string
	CandidateCacheTTL         time.Duration
	DecisionRateLimit         float64
	DecisionRateLimiter       string
	DecisionRateWindow        time.Duration
	DecisionBurst             int
	DecisionMaxInFlight       int
	DecisionQueueTimeout      time.Duration
	DecisionTimeout           time.Duration
	AgentProvider             string
	AgentBaseURL              string
	AgentAPIKey               string
	AgentModel                string
	AgentAPIStyle             string
	AgentTimeout              time.Duration
	AgentMaxRetries           int
	AgentFallbackEnabled      bool
	AgentCircuitFailures      int
	AgentCircuitOpen          time.Duration
	AgentMaxDailyBudgetFen    int64
	AgentMaxImpressionCostFen int64
	AgentMaxConditions        int
}

func Load() (Config, error) {
	cfg := Config{
		Environment:               envOr("ADFLOW_ENV", "local"),
		HTTPAddr:                  envOr("ADFLOW_HTTP_ADDR", ":18080"),
		MySQLDSN:                  envOr("ADFLOW_MYSQL_DSN", "adflow:adflow@tcp(127.0.0.1:3306)/adflow?parseTime=true&charset=utf8mb4&loc=Local"),
		RedisAddr:                 envOr("ADFLOW_REDIS_ADDR", "127.0.0.1:6379"),
		RedisPassword:             os.Getenv("ADFLOW_REDIS_PASSWORD"),
		ShutdownTimeout:           10 * time.Second,
		DependencyTimeout:         800 * time.Millisecond,
		CampaignRepository:        envOr("ADFLOW_CAMPAIGN_REPOSITORY", "memory"),
		ReservationAdapter:        envOr("ADFLOW_RESERVATION_ADAPTER", "memory"),
		DecisionStore:             envOr("ADFLOW_DECISION_STORE", "memory"),
		ProfileStore:              envOr("ADFLOW_PROFILE_STORE", "memory"),
		EventTransport:            envOr("ADFLOW_EVENT_TRANSPORT", "sync"),
		KafkaBrokers:              envOr("ADFLOW_KAFKA_BROKERS", "127.0.0.1:9092"),
		KafkaTopic:                envOr("ADFLOW_KAFKA_TOPIC", "adflow.ad-events.v1"),
		KafkaDeadLetterTopic:      envOr("ADFLOW_KAFKA_DEAD_LETTER_TOPIC", "adflow.ad-events.dlq.v1"),
		KafkaConsumerGroup:        envOr("ADFLOW_KAFKA_CONSUMER_GROUP", "adflow-metrics-v1"),
		JWTSecret:                 envOr("ADFLOW_JWT_SECRET", localJWTSecret),
		JWTIssuer:                 envOr("ADFLOW_JWT_ISSUER", "adflow"),
		AccessTokenTTL:            30 * time.Minute,
		AuthUsers:                 os.Getenv("ADFLOW_AUTH_USERS"),
		AuditStore:                envOr("ADFLOW_AUDIT_STORE", "memory"),
		CandidateCacheTTL:         5 * time.Second,
		DecisionRateLimit:         5000,
		DecisionRateLimiter:       envOr("ADFLOW_DECISION_RATE_LIMITER", "memory"),
		DecisionRateWindow:        time.Second,
		DecisionBurst:             1000,
		DecisionMaxInFlight:       256,
		DecisionQueueTimeout:      5 * time.Millisecond,
		DecisionTimeout:           100 * time.Millisecond,
		AgentProvider:             envOr("ADFLOW_AGENT_PROVIDER", "mock"),
		AgentBaseURL:              envOr("ADFLOW_AGENT_BASE_URL", "https://api.openai.com/v1"),
		AgentAPIKey:               os.Getenv("ADFLOW_AGENT_API_KEY"),
		AgentModel:                os.Getenv("ADFLOW_AGENT_MODEL"),
		AgentAPIStyle:             envOr("ADFLOW_AGENT_API_STYLE", "responses"),
		AgentTimeout:              8 * time.Second,
		AgentMaxRetries:           2,
		AgentFallbackEnabled:      true,
		AgentCircuitFailures:      3,
		AgentCircuitOpen:          30 * time.Second,
		AgentMaxDailyBudgetFen:    10_000_000,
		AgentMaxImpressionCostFen: 100_000,
		AgentMaxConditions:        20,
	}

	var err error
	if cfg.ShutdownTimeout, err = durationEnv("ADFLOW_SHUTDOWN_TIMEOUT", cfg.ShutdownTimeout); err != nil {
		return Config{}, err
	}
	if cfg.DependencyTimeout, err = durationEnv("ADFLOW_DEPENDENCY_TIMEOUT", cfg.DependencyTimeout); err != nil {
		return Config{}, err
	}
	if cfg.RedisDB, err = intEnv("ADFLOW_REDIS_DB", 0); err != nil {
		return Config{}, err
	}
	if cfg.AuthEnabled, err = boolEnv("ADFLOW_AUTH_ENABLED", false); err != nil {
		return Config{}, err
	}
	if cfg.AccessTokenTTL, err = durationEnv("ADFLOW_ACCESS_TOKEN_TTL", cfg.AccessTokenTTL); err != nil {
		return Config{}, err
	}
	if cfg.CandidateCacheTTL, err = durationEnv("ADFLOW_CANDIDATE_CACHE_TTL", cfg.CandidateCacheTTL); err != nil {
		return Config{}, err
	}
	if cfg.DecisionRateLimit, err = floatEnv("ADFLOW_DECISION_RATE_LIMIT", cfg.DecisionRateLimit); err != nil {
		return Config{}, err
	}
	if cfg.DecisionRateWindow, err = durationEnv("ADFLOW_DECISION_RATE_WINDOW", cfg.DecisionRateWindow); err != nil {
		return Config{}, err
	}
	if cfg.DecisionBurst, err = intEnv("ADFLOW_DECISION_BURST", cfg.DecisionBurst); err != nil {
		return Config{}, err
	}
	if cfg.DecisionMaxInFlight, err = intEnv("ADFLOW_DECISION_MAX_IN_FLIGHT", cfg.DecisionMaxInFlight); err != nil {
		return Config{}, err
	}
	if cfg.DecisionQueueTimeout, err = durationEnv("ADFLOW_DECISION_QUEUE_TIMEOUT", cfg.DecisionQueueTimeout); err != nil {
		return Config{}, err
	}
	if cfg.DecisionTimeout, err = durationEnv("ADFLOW_DECISION_TIMEOUT", cfg.DecisionTimeout); err != nil {
		return Config{}, err
	}
	if cfg.AgentTimeout, err = durationEnv("ADFLOW_AGENT_TIMEOUT", cfg.AgentTimeout); err != nil {
		return Config{}, err
	}
	if cfg.AgentMaxRetries, err = intEnv("ADFLOW_AGENT_MAX_RETRIES", cfg.AgentMaxRetries); err != nil {
		return Config{}, err
	}
	if cfg.AgentFallbackEnabled, err = boolEnv("ADFLOW_AGENT_FALLBACK_ENABLED", cfg.AgentFallbackEnabled); err != nil {
		return Config{}, err
	}
	if cfg.AgentCircuitFailures, err = intEnv("ADFLOW_AGENT_CIRCUIT_FAILURES", cfg.AgentCircuitFailures); err != nil {
		return Config{}, err
	}
	if cfg.AgentCircuitOpen, err = durationEnv("ADFLOW_AGENT_CIRCUIT_OPEN", cfg.AgentCircuitOpen); err != nil {
		return Config{}, err
	}
	if cfg.AgentMaxDailyBudgetFen, err = int64Env("ADFLOW_AGENT_MAX_DAILY_BUDGET_FEN", cfg.AgentMaxDailyBudgetFen); err != nil {
		return Config{}, err
	}
	if cfg.AgentMaxImpressionCostFen, err = int64Env("ADFLOW_AGENT_MAX_IMPRESSION_COST_FEN", cfg.AgentMaxImpressionCostFen); err != nil {
		return Config{}, err
	}
	if cfg.AgentMaxConditions, err = intEnv("ADFLOW_AGENT_MAX_CONDITIONS", cfg.AgentMaxConditions); err != nil {
		return Config{}, err
	}
	if cfg.HTTPAddr == "" || cfg.MySQLDSN == "" || cfg.RedisAddr == "" {
		return Config{}, fmt.Errorf("http address, mysql dsn and redis address must not be empty")
	}
	if cfg.CampaignRepository != "memory" && cfg.CampaignRepository != "mysql" {
		return Config{}, fmt.Errorf("ADFLOW_CAMPAIGN_REPOSITORY must be memory or mysql: %q", cfg.CampaignRepository)
	}
	if cfg.ReservationAdapter != "memory" && cfg.ReservationAdapter != "redis" {
		return Config{}, fmt.Errorf("ADFLOW_RESERVATION_ADAPTER must be memory or redis: %q", cfg.ReservationAdapter)
	}
	if cfg.DecisionStore != "memory" && cfg.DecisionStore != "mysql" {
		return Config{}, fmt.Errorf("ADFLOW_DECISION_STORE must be memory or mysql: %q", cfg.DecisionStore)
	}
	if cfg.ProfileStore != "memory" && cfg.ProfileStore != "mysql" {
		return Config{}, fmt.Errorf("ADFLOW_PROFILE_STORE must be memory or mysql: %q", cfg.ProfileStore)
	}
	if cfg.EventTransport != "sync" && cfg.EventTransport != "kafka" {
		return Config{}, fmt.Errorf("ADFLOW_EVENT_TRANSPORT must be sync or kafka: %q", cfg.EventTransport)
	}
	if cfg.EventTransport == "kafka" && (cfg.KafkaBrokers == "" || cfg.KafkaTopic == "" || cfg.KafkaDeadLetterTopic == "" || cfg.KafkaConsumerGroup == "") {
		return Config{}, fmt.Errorf("kafka brokers, topic and consumer group must not be empty")
	}
	if cfg.AuditStore != "memory" && cfg.AuditStore != "mysql" {
		return Config{}, fmt.Errorf("ADFLOW_AUDIT_STORE must be memory or mysql: %q", cfg.AuditStore)
	}
	if len(cfg.JWTSecret) < 32 || cfg.JWTIssuer == "" {
		return Config{}, fmt.Errorf("JWT secret must contain at least 32 characters and issuer must not be empty")
	}
	if cfg.AuthEnabled && cfg.Environment != "local" && cfg.Environment != "test" && cfg.AuthUsers == "" {
		return Config{}, fmt.Errorf("ADFLOW_AUTH_USERS is required when authentication is enabled outside local and test environments")
	}
	if cfg.AuthEnabled && cfg.Environment != "local" && cfg.Environment != "test" && cfg.JWTSecret == localJWTSecret {
		return Config{}, fmt.Errorf("ADFLOW_JWT_SECRET must be replaced outside local and test environments")
	}
	if cfg.DecisionBurst <= 0 || cfg.DecisionMaxInFlight <= 0 {
		return Config{}, fmt.Errorf("decision burst and max in-flight limits must be positive")
	}
	if cfg.DecisionRateLimiter != "memory" && cfg.DecisionRateLimiter != "redis" {
		return Config{}, fmt.Errorf("ADFLOW_DECISION_RATE_LIMITER must be memory or redis: %q", cfg.DecisionRateLimiter)
	}
	if cfg.DecisionRateWindow < time.Millisecond {
		return Config{}, fmt.Errorf("ADFLOW_DECISION_RATE_WINDOW must be at least 1ms")
	}
	if cfg.DecisionTimeout <= cfg.DecisionQueueTimeout {
		return Config{}, fmt.Errorf("ADFLOW_DECISION_TIMEOUT must exceed ADFLOW_DECISION_QUEUE_TIMEOUT")
	}
	if cfg.AgentProvider != "mock" && cfg.AgentProvider != "openai-compatible" {
		return Config{}, fmt.Errorf("ADFLOW_AGENT_PROVIDER must be mock or openai-compatible: %q", cfg.AgentProvider)
	}
	if cfg.AgentAPIStyle != "responses" && cfg.AgentAPIStyle != "chat_completions" {
		return Config{}, fmt.Errorf("ADFLOW_AGENT_API_STYLE must be responses or chat_completions: %q", cfg.AgentAPIStyle)
	}
	if cfg.AgentProvider == "openai-compatible" && (cfg.AgentAPIKey == "" || cfg.AgentModel == "") {
		return Config{}, fmt.Errorf("ADFLOW_AGENT_API_KEY and ADFLOW_AGENT_MODEL are required for the OpenAI-compatible provider")
	}
	if cfg.AgentMaxRetries > 5 || cfg.AgentCircuitFailures <= 0 || cfg.AgentMaxConditions <= 0 || cfg.AgentMaxConditions > 50 ||
		cfg.AgentMaxDailyBudgetFen <= 0 || cfg.AgentMaxImpressionCostFen <= 0 || cfg.AgentMaxDailyBudgetFen < cfg.AgentMaxImpressionCostFen {
		return Config{}, fmt.Errorf("Agent retry, circuit, condition, and budget limits are invalid")
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func durationEnv(key string, fallback time.Duration) (time.Duration, error) {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration: %q", key, value)
	}
	return parsed, nil
}

func intEnv(key string, fallback int) (int, error) {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer: %q", key, value)
	}
	return parsed, nil
}

func boolEnv(key string, fallback bool) (bool, error) {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean: %q", key, value)
	}
	return parsed, nil
}

func floatEnv(key string, fallback float64) (float64, error) {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive number: %q", key, value)
	}
	return parsed, nil
}

func int64Env(key string, fallback int64) (int64, error) {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer: %q", key, value)
	}
	return parsed, nil
}
