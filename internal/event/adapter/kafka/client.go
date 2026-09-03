package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/zhanghaiyang/adflow/internal/event/domain"
)

const SchemaVersion = 1

type Message struct {
	SchemaVersion int          `json:"schemaVersion"`
	Event         domain.Event `json:"event"`
}

type Publisher struct {
	client          *kgo.Client
	topic           string
	deadLetterTopic string
}

func NewPublisher(brokers []string, topic, deadLetterTopic string) (*Publisher, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.RecordRetries(5),
	)
	if err != nil {
		return nil, err
	}
	return &Publisher{client: client, topic: topic, deadLetterTopic: deadLetterTopic}, nil
}

func (p *Publisher) PublishEvent(ctx context.Context, event domain.Event) error {
	value, err := json.Marshal(Message{SchemaVersion: SchemaVersion, Event: event})
	if err != nil {
		return err
	}
	return p.publish(ctx, p.topic, event.RequestID, value)
}

type DeadLetterMessage struct {
	SchemaVersion int          `json:"schemaVersion"`
	Event         domain.Event `json:"event"`
	Error         string       `json:"error"`
	Attempts      int          `json:"attempts"`
	FailedAt      time.Time    `json:"failedAt"`
}

func (p *Publisher) PublishDeadLetter(ctx context.Context, event domain.Event, message string, attempts int, failedAt time.Time) error {
	value, err := json.Marshal(DeadLetterMessage{
		SchemaVersion: SchemaVersion, Event: event, Error: message, Attempts: attempts, FailedAt: failedAt,
	})
	if err != nil {
		return err
	}
	return p.publish(ctx, p.deadLetterTopic, event.RequestID, value)
}

func (p *Publisher) publish(ctx context.Context, topic, key string, value []byte) error {
	record := &kgo.Record{Topic: topic, Key: []byte(key), Value: value}
	if result := p.client.ProduceSync(ctx, record).FirstErr(); result != nil {
		return fmt.Errorf("publish kafka record to %s: %w", topic, result)
	}
	return nil
}

func (p *Publisher) Close() { p.client.Close() }

type Consumer struct {
	client   *kgo.Client
	topic    string
	logger   *slog.Logger
	observer interface {
		SetKafkaConsumerLag(string, int32, int64)
	}
}

func NewConsumer(brokers []string, topic, group string, logger *slog.Logger, observer interface {
	SetKafkaConsumerLag(string, int32, int64)
}) (*Consumer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topic),
		kgo.DisableAutoCommit(),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		return nil, err
	}
	return &Consumer{client: client, topic: topic, logger: logger, observer: observer}, nil
}

func (c *Consumer) Run(ctx context.Context, handle func(context.Context, domain.Event) error) error {
	for ctx.Err() == nil {
		fetches := c.client.PollFetches(ctx)
		if err := fetches.Err(); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			c.logger.Error("poll kafka events", "error", err)
			continue
		}
		highWatermarks := make(map[string]int64)
		fetches.EachPartition(func(partition kgo.FetchTopicPartition) {
			highWatermarks[fmt.Sprintf("%s:%d", partition.Topic, partition.Partition)] = partition.HighWatermark
		})
		iter := fetches.RecordIter()
		for !iter.Done() {
			record := iter.Next()
			if c.observer != nil {
				high := highWatermarks[fmt.Sprintf("%s:%d", record.Topic, record.Partition)]
				lag := max(high-record.Offset-1, 0)
				c.observer.SetKafkaConsumerLag(record.Topic, record.Partition, lag)
			}
			message, err := Decode(record.Value)
			if err != nil {
				c.logger.Error("decode kafka event", "error", err, "partition", record.Partition, "offset", record.Offset)
				continue
			}
			if err := handle(ctx, message.Event); err != nil {
				c.logger.Error("process kafka event", "error", err, "event_id", message.Event.EventID)
				continue
			}
			if err := c.client.CommitRecords(ctx, record); err != nil {
				c.logger.Error("commit kafka event", "error", err, "partition", record.Partition, "offset", record.Offset)
			}
		}
	}
	return nil
}

func (c *Consumer) Close() { c.client.Close() }

func Decode(value []byte) (Message, error) {
	var message Message
	if err := json.Unmarshal(value, &message); err != nil {
		return Message{}, err
	}
	if message.SchemaVersion != SchemaVersion || message.Event.EventID == "" || message.Event.RequestID == "" {
		return Message{}, fmt.Errorf("unsupported or invalid ad event message")
	}
	return message, nil
}
