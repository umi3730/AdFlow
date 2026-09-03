package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	agenthttp "github.com/zhanghaiyang/adflow/internal/agentassistant/adapter/http"
	agentmock "github.com/zhanghaiyang/adflow/internal/agentassistant/adapter/mock"
	agentapp "github.com/zhanghaiyang/adflow/internal/agentassistant/application"
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
	candidateProvider := decisioncampaign.NewProvider(campaignRepository, campaignRepository)
	decisionService := decisionapp.NewService(candidateProvider, profileStore, reservations, reservations, decisionStore)
	decisionHandler := decisionhttp.NewHandler(decisionService, profileStore, metrics)
	var eventService interface {
		Record(context.Context, eventdomain.Event) (bool, error)
		Metrics(context.Context, string) (eventdomain.Metrics, error)
	}
	asyncEvents := cfg.EventTransport == "kafka"
	if asyncEvents {
		eventStore := eventmysql.NewStore(db)
		eventProcessor := eventapp.NewService(eventStore, decisionStore, reservations)
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
		eventService = eventapp.NewAsyncService(eventProcessor, decisionStore, outbox)
		relay := eventapp.NewOutboxRelay(outbox, publisher, publisher, metrics)
		go func() {
			if relayErr := relay.Run(ctx); relayErr != nil {
				logger.Error("outbox relay stopped", "error", relayErr)
			}
		}()
		go func() {
			if consumeErr := consumer.Run(ctx, func(processCtx context.Context, event eventdomain.Event) error {
				_, processErr := eventProcessor.Record(processCtx, event)
				return processErr
			}); consumeErr != nil {
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
	agentService := agentapp.NewService(agentmock.NewProvider())
	agentHandler := agenthttp.NewHandler(agentService)
	server := httptransport.NewServer(cfg.HTTPAddr, cfg.Environment, logger, healthService, metrics, campaignHandler, decisionHandler, eventHandler, agentHandler)

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
