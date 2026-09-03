package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
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
		kgo.BlockRebalanceOnPoll(),
	)
	if err != nil {
		return nil, err
	}
	return &Consumer{client: client, topic: topic, logger: logger, observer: observer}, nil
}

func (c *Consumer) Run(ctx context.Context, handle func(context.Context, domain.Event) error) error {
	for ctx.Err() == nil {
		fetches := c.client.PollRecords(ctx, 300)
		if err := fetches.Err(); err != nil {
			c.client.AllowRebalance()
			if ctx.Err() != nil {
				return nil
			}
			c.logger.Error("poll kafka events", "error", err)
			continue
		}
		highWatermarks := make(map[string]int64)
		partitions := make([]kgo.FetchTopicPartition, 0)
		fetches.EachPartition(func(partition kgo.FetchTopicPartition) {
			highWatermarks[fmt.Sprintf("%s:%d", partition.Topic, partition.Partition)] = partition.HighWatermark
			partitions = append(partitions, partition)
		})
		results := make(chan partitionResult, len(partitions))
		var wait sync.WaitGroup
		for _, partition := range partitions {
			wait.Add(1)
			go func(partition kgo.FetchTopicPartition) {
				defer wait.Done()
				results <- c.processPartition(ctx, partition, highWatermarks, handle)
			}(partition)
		}
		wait.Wait()
		close(results)
		commits := make([]*kgo.Record, 0, len(partitions))
		retries := make([]*kgo.Record, 0, len(partitions))
		for result := range results {
			if result.commit != nil {
				commits = append(commits, result.commit)
			}
			if result.retry != nil {
				retries = append(retries, result.retry)
			}
		}
		if len(commits) > 0 {
			if err := c.client.CommitRecords(ctx, commits...); err != nil {
				c.logger.Error("commit Kafka event batch", "error", err, "partitions", len(commits))
				retries = append(retries, commits...)
			}
		}
		if len(retries) > 0 && ctx.Err() == nil {
			c.seekForRetry(retries)
		}
		c.client.AllowRebalance()
	}
	return nil
}

type partitionResult struct {
	commit *kgo.Record
	retry  *kgo.Record
}

func (c *Consumer) processPartition(ctx context.Context, partition kgo.FetchTopicPartition, highWatermarks map[string]int64, handle func(context.Context, domain.Event) error) partitionResult {
	var result partitionResult
	for _, record := range partition.Records {
		if c.observer != nil {
			high := highWatermarks[fmt.Sprintf("%s:%d", record.Topic, record.Partition)]
			lag := max(high-record.Offset-1, 0)
			c.observer.SetKafkaConsumerLag(record.Topic, record.Partition, lag)
		}
		message, err := Decode(record.Value)
		if err != nil {
			// Invalid envelopes cannot become valid through retry. Advance the
			// partition and keep the error visible instead of blocking it forever.
			c.logger.Error("decode Kafka event", "error", err, "partition", record.Partition, "offset", record.Offset)
			result.commit = record
			continue
		}
		if err := handle(ctx, message.Event); err != nil {
			c.logger.Error("process Kafka event", "error", err, "event_id", message.Event.EventID, "partition", record.Partition, "offset", record.Offset)
			result.retry = record
			break
		}
		result.commit = record
	}
	return result
}

func (c *Consumer) seekForRetry(records []*kgo.Record) {
	offsets := make(map[string]map[int32]kgo.EpochOffset)
	for _, record := range records {
		partitions := offsets[record.Topic]
		if partitions == nil {
			partitions = make(map[int32]kgo.EpochOffset)
			offsets[record.Topic] = partitions
		}
		current, exists := partitions[record.Partition]
		if !exists || record.Offset < current.Offset {
			partitions[record.Partition] = kgo.EpochOffset{Epoch: record.LeaderEpoch, Offset: record.Offset}
		}
	}
	c.client.SetOffsets(offsets)
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
