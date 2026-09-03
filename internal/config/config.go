package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Environment          string
	HTTPAddr             string
	ShutdownTimeout      time.Duration
	DependencyTimeout    time.Duration
	MySQLDSN             string
	RedisAddr            string
	RedisPassword        string
	RedisDB              int
	CampaignRepository   string
	ReservationAdapter   string
	DecisionStore        string
	ProfileStore         string
	EventTransport       string
	KafkaBrokers         string
	KafkaTopic           string
	KafkaDeadLetterTopic string
	KafkaConsumerGroup   string
}

func Load() (Config, error) {
	cfg := Config{
		Environment:          envOr("ADFLOW_ENV", "local"),
		HTTPAddr:             envOr("ADFLOW_HTTP_ADDR", ":18080"),
		MySQLDSN:             envOr("ADFLOW_MYSQL_DSN", "adflow:adflow@tcp(127.0.0.1:3306)/adflow?parseTime=true&charset=utf8mb4&loc=Local"),
		RedisAddr:            envOr("ADFLOW_REDIS_ADDR", "127.0.0.1:6379"),
		RedisPassword:        os.Getenv("ADFLOW_REDIS_PASSWORD"),
		ShutdownTimeout:      10 * time.Second,
		DependencyTimeout:    800 * time.Millisecond,
		CampaignRepository:   envOr("ADFLOW_CAMPAIGN_REPOSITORY", "memory"),
		ReservationAdapter:   envOr("ADFLOW_RESERVATION_ADAPTER", "memory"),
		DecisionStore:        envOr("ADFLOW_DECISION_STORE", "memory"),
		ProfileStore:         envOr("ADFLOW_PROFILE_STORE", "memory"),
		EventTransport:       envOr("ADFLOW_EVENT_TRANSPORT", "sync"),
		KafkaBrokers:         envOr("ADFLOW_KAFKA_BROKERS", "127.0.0.1:9092"),
		KafkaTopic:           envOr("ADFLOW_KAFKA_TOPIC", "adflow.ad-events.v1"),
		KafkaDeadLetterTopic: envOr("ADFLOW_KAFKA_DEAD_LETTER_TOPIC", "adflow.ad-events.dlq.v1"),
		KafkaConsumerGroup:   envOr("ADFLOW_KAFKA_CONSUMER_GROUP", "adflow-metrics-v1"),
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
