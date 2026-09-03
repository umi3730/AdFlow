package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	agenthttp "github.com/zhanghaiyang/adflow/internal/agentassistant/adapter/http"
	agentmock "github.com/zhanghaiyang/adflow/internal/agentassistant/adapter/mock"
	agentopenai "github.com/zhanghaiyang/adflow/internal/agentassistant/adapter/openai"
	agentapp "github.com/zhanghaiyang/adflow/internal/agentassistant/application"
	agentdomain "github.com/zhanghaiyang/adflow/internal/agentassistant/domain"
	audithttp "github.com/zhanghaiyang/adflow/internal/audit/adapter/http"
	auditmemory "github.com/zhanghaiyang/adflow/internal/audit/adapter/memory"
	auditmysql "github.com/zhanghaiyang/adflow/internal/audit/adapter/mysql"
	auditapp "github.com/zhanghaiyang/adflow/internal/audit/application"
	auditdomain "github.com/zhanghaiyang/adflow/internal/audit/domain"
	campaignhttp "github.com/zhanghaiyang/adflow/internal/campaign/adapter/http"
	"github.com/zhanghaiyang/adflow/internal/campaign/adapter/memory"
	campaignmysql "github.com/zhanghaiyang/adflow/internal/campaign/adapter/mysql"
	"github.com/zhanghaiyang/adflow/internal/campaign/application"
	"github.com/zhanghaiyang/adflow/internal/campaign/domain"
	"github.com/zhanghaiyang/adflow/internal/config"
	decisioncampaign "github.com/zhanghaiyang/adflow/internal/decision/adapter/campaign"
	decisionhttp "github.com/zhanghaiyang/adflow/internal/decision/adapter/http"
	decisionmemory "github.com/zhanghaiyang/adflow/internal/decision/adapter/memory"
	decisionmysql "github.com/zhanghaiyang/adflow/internal/decision/adapter/mysql"
	decisionredis "github.com/zhanghaiyang/adflow/internal/decision/adapter/redis"
	decisionapp "github.com/zhanghaiyang/adflow/internal/decision/application"
	decisiondomain "github.com/zhanghaiyang/adflow/internal/decision/domain"
	eventhttp "github.com/zhanghaiyang/adflow/internal/event/adapter/http"
	eventkafka "github.com/zhanghaiyang/adflow/internal/event/adapter/kafka"
	eventmemory "github.com/zhanghaiyang/adflow/internal/event/adapter/memory"
	eventmysql "github.com/zhanghaiyang/adflow/internal/event/adapter/mysql"
	eventapp "github.com/zhanghaiyang/adflow/internal/event/application"
	eventdomain "github.com/zhanghaiyang/adflow/internal/event/domain"
	"github.com/zhanghaiyang/adflow/internal/health"
	identityhttp "github.com/zhanghaiyang/adflow/internal/identity/adapter/http"
	identityjwt "github.com/zhanghaiyang/adflow/internal/identity/adapter/jwt"
	identitymemory "github.com/zhanghaiyang/adflow/internal/identity/adapter/memory"
	identitypassword "github.com/zhanghaiyang/adflow/internal/identity/adapter/password"
	identityapp "github.com/zhanghaiyang/adflow/internal/identity/application"
	"github.com/zhanghaiyang/adflow/internal/observability"
	"github.com/zhanghaiyang/adflow/internal/platform/cache"
	"github.com/zhanghaiyang/adflow/internal/platform/database"
	httptransport "github.com/zhanghaiyang/adflow/internal/transport/http"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load configuration", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := database.OpenMySQL(cfg.MySQLDSN)
	if err != nil {
		logger.Error("initialize mysql", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	redisClient := cache.NewRedis(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	defer redisClient.Close()

	healthService := health.NewService(cfg.DependencyTimeout, map[string]health.Checker{
		"mysql": db,
		"redis": redisClient,
	})
	metrics := observability.New()
	userStore, err := identitymemory.NewUserStore(cfg.AuthUsers, cfg.Environment, cfg.AuthEnabled)
	if err != nil {
		logger.Error("initialize authentication users", "error", err)
		os.Exit(1)
	}
	tokenManager, err := identityjwt.NewManager(cfg.JWTSecret, cfg.JWTIssuer, cfg.AccessTokenTTL)
	if err != nil {
		logger.Error("initialize JWT manager", "error", err)
		os.Exit(1)
	}
	identityService := identityapp.NewService(userStore, identitypassword.Bcrypt{}, tokenManager)
	identityHandler := identityhttp.NewHandler(identityService, cfg.AuthEnabled)
	logger.Info("authentication configured", "enabled", cfg.AuthEnabled)
	var auditStore auditdomain.Store
	if cfg.AuditStore == "mysql" {
		auditStore = auditmysql.NewStore(db)
	} else {
		auditStore = auditmemory.NewStore()
	}
	auditService := auditapp.NewService(auditStore)
	auditHandler := audithttp.NewHandler(auditService)
	logger.Info("audit store selected", "adapter", cfg.AuditStore)
	var campaignRepository interface {
		domain.Repository
		domain.CreativeRepository
	}
	switch cfg.CampaignRepository {
	case "mysql":
		campaignRepository = campaignmysql.NewRepository(db)
	default:
		campaignRepository = memory.NewRepository()
	}
	logger.Info("campaign repository selected", "adapter", cfg.CampaignRepository)
	campaignService := application.NewService(campaignRepository, nil)
	creativeService := application.NewCreativeService(campaignRepository, campaignRepository)
	campaignHandler := campaignhttp.NewHandler(campaignService, creativeService)
	decisionRuntime := decisionmemory.NewRuntime()
	var decisionStore decisiondomain.DecisionStore
	if cfg.DecisionStore == "mysql" {
		decisionStore = decisionmysql.NewStore(db)
	} else {
		decisionStore = decisionRuntime
	}
	logger.Info("decision store selected", "adapter", cfg.DecisionStore)
	var profileStore decisiondomain.ProfileStore
	if cfg.ProfileStore == "mysql" {
		profileStore = decisionmysql.NewProfileStore(db)
	} else {
		profileStore = decisionRuntime
	}
	logger.Info("profile store selected", "adapter", cfg.ProfileStore)
	var reservations interface {
		decisiondomain.FrequencyGate
		decisiondomain.BudgetGate
	}
	if cfg.ReservationAdapter == "redis" {
		reservations = decisionredis.NewReservations(redisClient.Client(), "adflow")
	} else {
		reservations = decisionRuntime
	}
	logger.Info("reservation adapter selected", "adapter", cfg.ReservationAdapter)
	candidateSource := decisioncampaign.NewProvider(campaignRepository, campaignRepository)
	candidateProvider, err := decisioncampaign.NewCachedProvider(candidateSource, cfg.CandidateCacheTTL, metrics)
	if err != nil {
		logger.Error("initialize candidate cache", "error", err)
		os.Exit(1)
	}
	logger.Info("candidate cache configured", "ttl", cfg.CandidateCacheTTL)
	decisionService := decisionapp.NewService(candidateProvider, profileStore, reservations, reservations, decisionStore)
	var decisionRateLimiter decisionapp.RateLimiter
	if cfg.DecisionRateLimiter == "redis" {
		windowLimit := int(math.Ceil(cfg.DecisionRateLimit * cfg.DecisionRateWindow.Seconds()))
		decisionRateLimiter, err = decisionredis.NewSlidingWindowLimiter(redisClient.Client(), "adflow:admission:decision", windowLimit, cfg.DecisionRateWindow)
	} else {
		decisionRateLimiter, err = decisionapp.NewTokenBucketLimiter(cfg.DecisionRateLimit, cfg.DecisionBurst)
	}
	if err != nil {
		logger.Error("initialize decision rate limiter", "error", err)
		os.Exit(1)
	}
	admittedDecisionService, err := decisionapp.NewAdmissionService(decisionService, decisionRateLimiter, decisionapp.AdmissionConfig{
		MaxInFlight:  cfg.DecisionMaxInFlight,
		QueueTimeout: cfg.DecisionQueueTimeout, RequestTimeout: cfg.DecisionTimeout,
	}, metrics)
	if err != nil {
		logger.Error("initialize decision admission control", "error", err)
		os.Exit(1)
	}
	logger.Info("decision admission configured",
		"rate_limiter", cfg.DecisionRateLimiter, "rate_per_second", cfg.DecisionRateLimit,
		"window", cfg.DecisionRateWindow, "memory_burst", cfg.DecisionBurst,
		"max_in_flight", cfg.DecisionMaxInFlight, "queue_timeout", cfg.DecisionQueueTimeout,
		"request_timeout", cfg.DecisionTimeout,
	)
	decisionHandler := decisionhttp.NewHandler(admittedDecisionService, profileStore, metrics)
	var eventService interface {
		Record(context.Context, eventdomain.Event) (bool, error)
		Metrics(context.Context, string) (eventdomain.Metrics, error)
	}
	asyncEvents := cfg.EventTransport == "kafka"
	if asyncEvents {
		eventStore := eventmysql.NewStore(db)
		// Kafka ingestion settles reservations after the durable Outbox write.
		// The consumer therefore persists trusted, already-settled events in batches.
		eventProcessor := eventapp.NewService(eventStore, decisionStore, nil)
		outbox := eventmysql.NewOutbox(db, fmt.Sprintf("relay-%d-%d", os.Getpid(), time.Now().UnixNano()))
		brokers := strings.Split(cfg.KafkaBrokers, ",")
		publisher, kafkaErr := eventkafka.NewPublisher(brokers, cfg.KafkaTopic, cfg.KafkaDeadLetterTopic)
		if kafkaErr != nil {
			logger.Error("initialize kafka publisher", "error", kafkaErr)
			os.Exit(1)
		}
		defer publisher.Close()
		consumer, kafkaErr := eventkafka.NewConsumer(brokers, cfg.KafkaTopic, cfg.KafkaConsumerGroup, logger, metrics)
		if kafkaErr != nil {
			logger.Error("initialize kafka consumer", "error", kafkaErr)
			os.Exit(1)
		}
		defer consumer.Close()
		eventService = eventapp.NewAsyncService(eventProcessor, decisionStore, outbox, reservations)
		relay := eventapp.NewOutboxRelay(outbox, publisher, publisher, metrics)
		go func() {
			if relayErr := relay.Run(ctx); relayErr != nil {
				logger.Error("outbox relay stopped", "error", relayErr)
			}
		}()
		go func() {
			if consumeErr := consumer.RunBatch(ctx, eventProcessor.RecordBatch); consumeErr != nil {
				logger.Error("kafka consumer stopped", "error", consumeErr)
			}
		}()
	} else {
		eventStore := eventmemory.NewStore()
		eventProcessor := eventapp.NewService(eventStore, decisionStore, reservations)
		eventService = eventProcessor
	}
	logger.Info("event transport selected", "adapter", cfg.EventTransport)
	eventHandler := eventhttp.NewHandler(eventService, metrics, asyncEvents)
	var agentProvider agentdomain.Provider = agentmock.NewProvider()
	if cfg.AgentProvider == "openai-compatible" {
		primaryProvider, providerErr := agentopenai.NewProvider(agentopenai.ProviderConfig{
			BaseURL: cfg.AgentBaseURL, APIKey: cfg.AgentAPIKey, Model: cfg.AgentModel, APIStyle: cfg.AgentAPIStyle,
			Timeout: cfg.AgentTimeout, MaxRetries: cfg.AgentMaxRetries,
			MaxDailyBudgetFen: cfg.AgentMaxDailyBudgetFen, MaxImpressionCostFen: cfg.AgentMaxImpressionCostFen,
			MaxConditions: cfg.AgentMaxConditions,
		})
		if providerErr != nil {
			logger.Error("initialize Agent model provider", "error", providerErr)
			os.Exit(1)
		}
		agentProvider, providerErr = agentapp.NewResilientProvider(primaryProvider, agentmock.NewProvider(), agentapp.ResilienceConfig{
			PrimaryProvider: "openai-compatible", PrimaryModel: cfg.AgentModel,
			FailureThreshold: cfg.AgentCircuitFailures, OpenDuration: cfg.AgentCircuitOpen,
			FallbackEnabled: cfg.AgentFallbackEnabled,
		}, metrics)
		if providerErr != nil {
			logger.Error("initialize Agent provider resilience", "error", providerErr)
			os.Exit(1)
		}
	}
	logger.Info("Agent provider selected", "provider", cfg.AgentProvider, "model", cfg.AgentModel, "api_style", cfg.AgentAPIStyle)
	agentService := agentapp.NewServiceWithPolicy(agentProvider, agentapp.Policy{
		MaxDailyBudgetFen: cfg.AgentMaxDailyBudgetFen, MaxImpressionCostFen: cfg.AgentMaxImpressionCostFen,
		MaxConditions: cfg.AgentMaxConditions,
	})
	agentHandler := agenthttp.NewHandler(agentService)
	server := httptransport.NewServer(cfg.HTTPAddr, cfg.Environment, logger, healthService, metrics,
		identityHandler, auditHandler, campaignHandler, decisionHandler, eventHandler, agentHandler)

	errCh := make(chan error, 1)
	go func() {
		logger.Info("http server started", "addr", cfg.HTTPAddr, "environment", cfg.Environment)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err = <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server stopped unexpectedly", "error", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if shutdownErr := server.Shutdown(shutdownCtx); shutdownErr != nil {
		logger.Error("graceful shutdown failed", "error", shutdownErr)
		os.Exit(1)
	}
	logger.Info("shutdown complete")
}
