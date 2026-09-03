//go:build integration

package integration

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	decisionmysql "github.com/zhanghaiyang/adflow/internal/decision/adapter/mysql"
	decisionredis "github.com/zhanghaiyang/adflow/internal/decision/adapter/redis"
	decisiondomain "github.com/zhanghaiyang/adflow/internal/decision/domain"
	eventkafka "github.com/zhanghaiyang/adflow/internal/event/adapter/kafka"
	eventmysql "github.com/zhanghaiyang/adflow/internal/event/adapter/mysql"
	eventapp "github.com/zhanghaiyang/adflow/internal/event/application"
	eventdomain "github.com/zhanghaiyang/adflow/internal/event/domain"
)

func TestKafkaAckBeforeOutboxMarkProducesOneMetric(t *testing.T) {
	db := integrationMySQL(t)
	redisAddress := integrationEnv(t, "ADFLOW_IT_REDIS_ADDR")
	brokers := strings.Split(integrationEnv(t, "ADFLOW_IT_KAFKA_BROKERS"), ",")
	topic := integrationEnv(t, "ADFLOW_IT_KAFKA_TOPIC")
	deadLetterTopic := integrationEnv(t, "ADFLOW_IT_KAFKA_DEAD_LETTER_TOPIC")
	redisClient := redis.NewClient(&redis.Options{Addr: redisAddress, Protocol: 2, DisableIdentity: true})
	t.Cleanup(func() { _ = redisClient.Close() })
	if err := redisClient.Ping(t.Context()).Err(); err != nil {
		t.Fatal(err)
	}

	requestID := "req-" + newID(t)
	eventID := "evt-" + newID(t)
	campaignID := newID(t)
	creativeID := newID(t)
	userID := "user-" + newID(t)
	prefix := "adflow:it:kafka:" + newID(t)
	t.Cleanup(func() {
		deletePrefix(context.Background(), redisClient, prefix)
		_, _ = db.Exec(`DELETE FROM campaign_metrics WHERE campaign_id = ?`, campaignID)
		_, _ = db.Exec(`DELETE FROM processed_events WHERE event_id = ?`, eventID)
		_, _ = db.Exec(`DELETE FROM event_outbox WHERE event_id = ?`, eventID)
		_, _ = db.Exec(`DELETE FROM event_receipts WHERE event_id = ?`, eventID)
		_, _ = db.Exec(`DELETE FROM decisions WHERE request_id = ?`, requestID)
	})

	now := time.Now().UTC()
	reservations := decisionredis.NewReservations(redisClient, prefix)
	frequencyToken, allowed, err := reservations.ReserveFrequency(t.Context(), userID, campaignID, requestID, 3, now, 30*time.Second)
	if err != nil || !allowed {
		t.Fatalf("frequency reservation allowed=%v err=%v", allowed, err)
	}
	budgetToken, allowed, err := reservations.ReserveBudget(t.Context(), campaignID, 1000, 100, requestID, now, 30*time.Second)
	if err != nil || !allowed || frequencyToken != budgetToken {
		t.Fatalf("budget reservation token=%s allowed=%v err=%v", budgetToken, allowed, err)
	}
	decisionStore := decisionmysql.NewStore(db)
	if err := decisionStore.SaveDecision(t.Context(), decisiondomain.Result{
		RequestID: requestID, UserID: userID, SlotID: "integration-slot", Matched: true,
		CampaignID: campaignID, CreativeID: creativeID, ReservationToken: budgetToken,
		ExpiresAt: now.Add(30 * time.Second), Reason: decisiondomain.ReasonMatched,
	}); err != nil {
		t.Fatal(err)
	}
	persistedDecision, found, err := decisionStore.FindDecision(t.Context(), requestID)
	if err != nil || !found || persistedDecision.ReservationToken != requestID {
		t.Fatalf("persisted decision=%+v found=%v err=%v", persistedDecision, found, err)
	}
	budgetMetaKey := prefix + ":budget:rsv:" + requestID
	if exists, err := redisClient.Exists(t.Context(), budgetMetaKey).Result(); err != nil || exists != 1 {
		t.Fatalf("budget reservation metadata exists=%d err=%v", exists, err)
	}

	event := eventdomain.Event{
		EventID: eventID, RequestID: requestID, CampaignID: campaignID, CreativeID: creativeID,
		Type: eventdomain.Impression, OccurredAt: now,
	}
	outbox := eventmysql.NewOutbox(db, "recovery-relay-"+newID(t))
	eventStore := eventmysql.NewStore(db)
	processor := eventapp.NewService(eventStore, decisionStore, reservations)
	asyncService := eventapp.NewAsyncService(processor, decisionStore, outbox, reservations)
	created, err := asyncService.Record(t.Context(), event)
	if err != nil || !created {
		t.Fatalf("enqueue created=%v err=%v", created, err)
	}
	if spent, err := reservations.DebugBudgetSpent(t.Context(), campaignID, now); err != nil || spent != 100 {
		t.Fatalf("budget was not confirmed at durable ingestion: spent=%d err=%v", spent, err)
	}
	publisher, err := eventkafka.NewPublisher(brokers, topic, deadLetterTopic)
	if err != nil {
		t.Fatal(err)
	}
	defer publisher.Close()
	publishContext, publishCancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer publishCancel()

	// First publication represents the crash window: Kafka acknowledged the event,
	// but the process stopped before marking the Outbox row as published.
	if err := publisher.PublishEvent(publishContext, event); err != nil {
		t.Fatal(err)
	}
	relay := eventapp.NewOutboxRelay(outbox, publisher, publisher, nil)
	published, err := relay.RunOnce(publishContext)
	if err != nil || published != 1 {
		t.Fatalf("relay published=%d err=%v", published, err)
	}

	consumer, err := eventkafka.NewConsumer(brokers, topic, "adflow-it-"+newID(t), slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer consumer.Close()
	consumerContext, consumerCancel := context.WithCancel(t.Context())
	defer consumerCancel()
	consumerErrors := make(chan error, 1)
	var handled atomic.Int64
	var duplicates atomic.Int64
	var spentAfterCreated atomic.Int64
	go func() {
		consumerErrors <- consumer.Run(consumerContext, func(ctx context.Context, consumed eventdomain.Event) error {
			if consumed.EventID != eventID {
				return nil
			}
			wasCreated, processErr := processor.Record(ctx, consumed)
			if processErr == nil {
				handled.Add(1)
				if wasCreated {
					spent, _ := reservations.DebugBudgetSpent(ctx, campaignID, now)
					spentAfterCreated.Store(spent)
				} else {
					duplicates.Add(1)
				}
			}
			return processErr
		})
	}()

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		metrics, metricsErr := eventStore.Metrics(t.Context(), campaignID)
		if metricsErr != nil {
			t.Fatal(metricsErr)
		}
		if metrics.Impressions == 1 && handled.Load() >= 2 && duplicates.Load() >= 1 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	consumerCancel()
	select {
	case err := <-consumerErrors:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Kafka consumer did not stop after cancellation")
	}
	metrics, err := eventStore.Metrics(t.Context(), campaignID)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.Impressions != 1 || handled.Load() < 2 || duplicates.Load() < 1 {
		t.Fatalf("metrics=%+v handled=%d duplicates=%d", metrics, handled.Load(), duplicates.Load())
	}
	spent, err := reservations.DebugBudgetSpent(t.Context(), campaignID, now)
	if err != nil || spent != 100 {
		t.Fatalf("confirmed budget spent=%d after_created=%d meta_exists=%d err=%v", spent, spentAfterCreated.Load(), mustExists(t, redisClient, budgetMetaKey), err)
	}
	stats, err := outbox.Stats(t.Context())
	if err != nil || stats.Published < 1 {
		t.Fatalf("outbox stats=%+v err=%v", stats, err)
	}
}

func TestKafkaConsumerRestartReplaysUncommittedSideEffectSafely(t *testing.T) {
	db := integrationMySQL(t)
	brokers := strings.Split(integrationEnv(t, "ADFLOW_IT_KAFKA_BROKERS"), ",")
	topic := integrationEnv(t, "ADFLOW_IT_KAFKA_RESTART_TOPIC")
	deadLetterTopic := integrationEnv(t, "ADFLOW_IT_KAFKA_DEAD_LETTER_TOPIC")
	eventID := "evt-" + newID(t)
	campaignID := newID(t)
	event := eventdomain.Event{
		EventID: eventID, RequestID: "req-" + newID(t), CampaignID: campaignID,
		CreativeID: newID(t), Type: eventdomain.Impression, OccurredAt: time.Now().UTC(),
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM campaign_metrics WHERE campaign_id = ?`, campaignID)
		_, _ = db.Exec(`DELETE FROM processed_events WHERE event_id = ?`, eventID)
	})
	publisher, err := eventkafka.NewPublisher(brokers, topic, deadLetterTopic)
	if err != nil {
		t.Fatal(err)
	}
	defer publisher.Close()
	publishContext, publishCancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer publishCancel()
	if err := publisher.PublishEvent(publishContext, event); err != nil {
		t.Fatal(err)
	}

	store := eventmysql.NewStore(db)
	group := "adflow-it-restart-" + newID(t)
	firstConsumer := newTestConsumer(t, brokers, topic, group)
	firstContext, cancelFirst := context.WithCancel(t.Context())
	firstResult := make(chan bool, 1)
	firstDone := make(chan error, 1)
	injectedCrash := errors.New("injected crash after database commit and before offset commit")
	go func() {
		firstDone <- firstConsumer.Run(firstContext, func(ctx context.Context, consumed eventdomain.Event) error {
			if consumed.EventID != eventID {
				return nil
			}
			created, recordErr := store.Record(ctx, consumed)
			if recordErr != nil {
				return recordErr
			}
			select {
			case firstResult <- created:
			default:
			}
			return injectedCrash
		})
	}()
	if created := waitBool(t, firstResult, 12*time.Second); !created {
		t.Fatal("first consumer should create the database side effect")
	}
	cancelFirst()
	waitConsumer(t, firstDone)
	firstConsumer.Close()

	secondConsumer := newTestConsumer(t, brokers, topic, group)
	secondContext, cancelSecond := context.WithCancel(t.Context())
	secondResult := make(chan bool, 1)
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- secondConsumer.Run(secondContext, func(ctx context.Context, consumed eventdomain.Event) error {
			if consumed.EventID != eventID {
				return nil
			}
			created, recordErr := store.Record(ctx, consumed)
			if recordErr == nil {
				select {
				case secondResult <- created:
				default:
				}
			}
			return recordErr
		})
	}()
	if created := waitBool(t, secondResult, 12*time.Second); created {
		t.Fatal("replayed event should be absorbed by the eventId idempotency barrier")
	}
	time.Sleep(500 * time.Millisecond)
	cancelSecond()
	waitConsumer(t, secondDone)
	secondConsumer.Close()

	thirdConsumer := newTestConsumer(t, brokers, topic, group)
	thirdContext, cancelThird := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancelThird()
	thirdSeen := make(chan struct{}, 1)
	thirdDone := make(chan error, 1)
	go func() {
		thirdDone <- thirdConsumer.Run(thirdContext, func(_ context.Context, consumed eventdomain.Event) error {
			if consumed.EventID == eventID {
				thirdSeen <- struct{}{}
			}
			return nil
		})
	}()
	select {
	case <-thirdSeen:
		t.Fatal("committed event was delivered after the second consumer restart")
	case err := <-thirdDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("third consumer did not finish its bounded verification")
	}
	thirdConsumer.Close()
	metrics, err := store.Metrics(t.Context(), campaignID)
	if err != nil || metrics.Impressions != 1 {
		t.Fatalf("metrics=%+v err=%v", metrics, err)
	}
}

func newTestConsumer(t *testing.T, brokers []string, topic, group string) *eventkafka.Consumer {
	t.Helper()
	consumer, err := eventkafka.NewConsumer(brokers, topic, group, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	if err != nil {
		t.Fatal(err)
	}
	return consumer
}

func waitBool(t *testing.T, result <-chan bool, timeout time.Duration) bool {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(timeout):
		t.Fatal("timed out waiting for Kafka event")
		return false
	}
}

func waitConsumer(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Kafka consumer did not stop after cancellation")
	}
}

func mustExists(t *testing.T, client *redis.Client, key string) int64 {
	t.Helper()
	value, err := client.Exists(t.Context(), key).Result()
	if err != nil {
		t.Fatal(err)
	}
	return value
}
