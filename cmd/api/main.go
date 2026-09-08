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

	agenthttp "github.com/umi3730/adflow/internal/agentassistant/adapter/http"
	agentmock "github.com/umi3730/adflow/internal/agentassistant/adapter/mock"
	agentopenai "github.com/umi3730/adflow/internal/agentassistant/adapter/openai"
	agentapp "github.com/umi3730/adflow/internal/agentassistant/application"
	agentdomain "github.com/umi3730/adflow/internal/agentassistant/domain"
	audithttp "github.com/umi3730/adflow/internal/audit/adapter/http"
	auditmemory "github.com/umi3730/adflow/internal/audit/adapter/memory"
	auditmysql "github.com/umi3730/adflow/internal/audit/adapter/mysql"
	auditapp "github.com/umi3730/adflow/internal/audit/application"
	auditdomain "github.com/umi3730/adflow/internal/audit/domain"
	"github.com/umi3730/adflow/internal/bootstrap"
	campaignhttp "github.com/umi3730/adflow/internal/campaign/adapter/http"
	"github.com/umi3730/adflow/internal/campaign/adapter/memory"
	campaignmysql "github.com/umi3730/adflow/internal/campaign/adapter/mysql"
	"github.com/umi3730/adflow/internal/campaign/application"
	"github.com/umi3730/adflow/internal/campaign/domain"
	"github.com/umi3730/adflow/internal/config"
	decisioncampaign "github.com/umi3730/adflow/internal/decision/adapter/campaign"
	decisionhttp "github.com/umi3730/adflow/internal/decision/adapter/http"
	decisionmemory "github.com/umi3730/adflow/internal/decision/adapter/memory"
	decisionmysql "github.com/umi3730/adflow/internal/decision/adapter/mysql"
	decisionprofilecache "github.com/umi3730/adflow/internal/decision/adapter/profilecache"
	decisionredis "github.com/umi3730/adflow/internal/decision/adapter/redis"
	"github.com/umi3730/adflow/internal/decision/adapter/requestprofile"
	decisionapp "github.com/umi3730/adflow/internal/decision/application"
	decisiondomain "github.com/umi3730/adflow/internal/decision/domain"
	eventhttp "github.com/umi3730/adflow/internal/event/adapter/http"
	eventkafka "github.com/umi3730/adflow/internal/event/adapter/kafka"
	eventmemory "github.com/umi3730/adflow/internal/event/adapter/memory"
	eventmysql "github.com/umi3730/adflow/internal/event/adapter/mysql"
	eventapp "github.com/umi3730/adflow/internal/event/application"
	eventdomain "github.com/umi3730/adflow/internal/event/domain"
	"github.com/umi3730/adflow/internal/health"
	identityhttp "github.com/umi3730/adflow/internal/identity/adapter/http"
	identityjwt "github.com/umi3730/adflow/internal/identity/adapter/jwt"
	identitymemory "github.com/umi3730/adflow/internal/identity/adapter/memory"
	identitymysql "github.com/umi3730/adflow/internal/identity/adapter/mysql"
	identitypassword "github.com/umi3730/adflow/internal/identity/adapter/password"
	identityapp "github.com/umi3730/adflow/internal/identity/application"
	identitydomain "github.com/umi3730/adflow/internal/identity/domain"
	"github.com/umi3730/adflow/internal/observability"
	operationshttp "github.com/umi3730/adflow/internal/operations/adapter/http"
	operationsapp "github.com/umi3730/adflow/internal/operations/application"
	"github.com/umi3730/adflow/internal/platform/cache"
	"github.com/umi3730/adflow/internal/platform/database"
	httptransport "github.com/umi3730/adflow/internal/transport/http"
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
	var identityUsers identitydomain.UserRepository = userStore
	if cfg.AuthStore == "mysql" {
		identityUsers = identitymysql.NewUserStore(db, userStore)
	}
	identityService := identityapp.NewService(identityUsers, identitypassword.Bcrypt{}, tokenManager)
	if cfg.RegistrationEnabled {
		identityService.EnableRegistration(identitypassword.Bcrypt{})
	}
	identityHandler := identityhttp.NewHandler(identityService, cfg.AuthEnabled)
	identityHandler.SetDemoPrefill((cfg.Environment == "local" || cfg.Environment == "test") && strings.TrimSpace(cfg.AuthUsers) == "")
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
	var decisionInspector decisiondomain.DecisionInspector
	if cfg.DecisionStore == "mysql" {
		storedDecisions := decisionmysql.NewStore(db)
		decisionStore = storedDecisions
		decisionInspector = storedDecisions
	} else {
		decisionStore = decisionRuntime
		decisionInspector = decisionRuntime
	}
	logger.Info("decision store selected", "adapter", cfg.DecisionStore)
	var profileStore decisiondomain.ProfileStore
	if cfg.ProfileStore == "mysql" {
		profileStore = decisionmysql.NewProfileStore(db)
	} else if cfg.ProfileStore == "mysql-redis" {
		profileStore, err = decisionprofilecache.New(
			decisionmysql.NewProfileStore(db), redisClient.Client(), "adflow:profile",
			cfg.ProfileCacheTTL, cfg.ProfileNegativeCacheTTL, cfg.ProfileCacheTimeout, metrics,
		)
		if err != nil {
			logger.Error("initialize profile cache", "error", err)
			os.Exit(1)
		}
		logger.Info("profile cache configured", "ttl", cfg.ProfileCacheTTL, "negative_ttl", cfg.ProfileNegativeCacheTTL, "timeout", cfg.ProfileCacheTimeout)
	} else {
		profileStore = decisionRuntime
	}
	logger.Info("profile store selected", "adapter", cfg.ProfileStore)
	if cfg.DemoData {
		initializer, initErr := bootstrap.NewDemoInitializer(bootstrap.DemoOptions{
			DB: db, Campaigns: campaignRepository, Profiles: profileStore,
			PersistentCampaigns: cfg.CampaignRepository == "mysql",
			PersistentProfiles:  cfg.ProfileStore == "mysql" || cfg.ProfileStore == "mysql-redis",
		})
		if initErr != nil {
			logger.Error("configure demo data", "error", initErr)
			os.Exit(1)
		}
		initCtx, initCancel := context.WithTimeout(ctx, 15*time.Second)
		result, initErr := initializer.Run(initCtx, time.Now().UTC())
		if initErr == nil {
			initErr = initializer.RunAuctionDemo(initCtx, time.Now().UTC())
		}
		if initErr == nil && cfg.ProfileStore == "mysql-redis" {
			// Retry this exact invalidation on every startup: a previous startup
			// may have committed MySQL and failed before clearing negative cache.
			keys := make([]string, 0, len(bootstrap.DemoProfileIDs()))
			for _, userID := range append(bootstrap.DemoProfileIDs(), bootstrap.AuctionDemoProfileID) {
				keys = append(keys, "adflow:profile:"+userID)
			}
			initErr = redisClient.Client().Del(initCtx, keys...).Err()
		}
		initCancel()
		if initErr != nil {
			logger.Error("initialize demo data; apply pending migrations for persistent stores", "error", initErr)
			os.Exit(1)
		}
		logger.Info("demo data initialized", "campaign_installed", result.CampaignsInstalled, "profiles_installed", result.ProfilesInstalled)
	}
	var reservations interface {
		decisiondomain.FrequencyGate
		decisiondomain.BudgetGate
		decisiondomain.ImpressionSettler
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
	decisionService := decisionapp.NewService(candidateProvider, requestprofile.New(profileStore), reservations, reservations, decisionStore)
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
	simulationHandler := decisionhttp.NewSimulationHandler(admittedDecisionService, metrics, cfg.Environment)
	explanationHandler := decisionhttp.NewExplanationHandler(decisionapp.NewExplanationService(profileStore, candidateProvider))
	var eventService interface {
		Record(context.Context, eventdomain.Event) (bool, error)
		Metrics(context.Context, string) (eventdomain.Metrics, error)
	}
	asyncEvents := cfg.EventTransport == "kafka"
	var operationsStore eventdomain.OperationsStore
	var requestStateReader eventdomain.RequestStateReader
	if asyncEvents {
		eventStore := eventmysql.NewStore(db)
		// Durable settlement gates publication, and the consumer verifies its proof.
		eventProcessor := eventapp.NewService(eventStore, decisionStore, nil)
		outbox := eventmysql.NewOutbox(db, fmt.Sprintf("relay-%d-%d", os.Getpid(), time.Now().UnixNano()))
		if cfg.AsyncBackpressure {
			gate, gateErr := decisionapp.NewBackpressureGate(outbox, decisionapp.BackpressureConfig{
				SettlementHigh: int64(cfg.SettlementBacklogHigh), SettlementLow: int64(cfg.SettlementBacklogLow),
				OutboxHigh: int64(cfg.OutboxBacklogHigh), OutboxLow: int64(cfg.OutboxBacklogLow),
			}, metrics)
			if gateErr != nil {
				logger.Error("initialize asynchronous backpressure", "error", gateErr)
				os.Exit(1)
			}
			decisionService.SetNewDecisionGate(gate)
			go gate.Run(ctx)
			logger.Info("asynchronous backpressure enabled", "settlement_high", cfg.SettlementBacklogHigh, "settlement_low", cfg.SettlementBacklogLow, "outbox_high", cfg.OutboxBacklogHigh, "outbox_low", cfg.OutboxBacklogLow)
		}
		operationsStore = outbox
		requestStateReader = outbox
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
		eventService = eventapp.NewAsyncService(eventProcessor, decisionStore, outbox)
		settlementWorker := eventapp.NewSettlementWorker(outbox, reservations, logger)
		go func() {
			if err := settlementWorker.Run(ctx); err != nil {
				logger.Error("settlement worker stopped", "error", err)
			}
		}()
		relay := eventapp.NewOutboxRelay(outbox, publisher, publisher, metrics)
		go func() {
			if relayErr := relay.Run(ctx); relayErr != nil {
				logger.Error("outbox relay stopped", "error", relayErr)
			}
		}()
		go func() {
			if consumeErr := consumer.RunBatch(ctx, eventapp.RecordSettledBatch(outbox, eventProcessor)); consumeErr != nil {
				logger.Error("kafka consumer stopped", "error", consumeErr)
			}
		}()
	} else {
		eventStore := eventmemory.NewStore()
		requestStateReader = eventStore
		eventProcessor := eventapp.NewService(eventStore, decisionStore, reservations)
		eventService = eventProcessor
	}
	logger.Info("event transport selected", "adapter", cfg.EventTransport)
	eventHandler := eventhttp.NewHandler(eventService, metrics, asyncEvents)
	operationsHandler := operationshttp.NewHandler(operationsapp.NewService(operationsStore, metrics), operationsapp.NewTraceService(decisionInspector, requestStateReader, cfg.EventTransport))
	var agentProvider agentdomain.Provider = agentmock.NewProvider()
	if cfg.AgentProvider == "openai-compatible" {
		primaryProvider, providerErr := agentopenai.NewProvider(agentopenai.ProviderConfig{
			BaseURL: cfg.AgentBaseURL, APIKey: cfg.AgentAPIKey, Model: cfg.AgentModel, APIStyle: cfg.AgentAPIStyle, ThinkingMode: cfg.AgentThinkingMode,
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
	logger.Info("Agent provider selected", "provider", cfg.AgentProvider, "model", cfg.AgentModel, "api_style", cfg.AgentAPIStyle, "thinking", cfg.AgentThinkingMode)
	agentService := agentapp.NewServiceWithPolicy(agentProvider, agentapp.Policy{
		MaxDailyBudgetFen: cfg.AgentMaxDailyBudgetFen, MaxImpressionCostFen: cfg.AgentMaxImpressionCostFen,
		MaxConditions: cfg.AgentMaxConditions,
	})
	agentHandler := agenthttp.NewHandler(agentService)
	server := httptransport.NewServer(cfg.HTTPAddr, cfg.Environment, logger, healthService, metrics,
		identityHandler, auditHandler, campaignHandler, decisionHandler, simulationHandler, explanationHandler, eventHandler, operationsHandler, agentHandler)

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
