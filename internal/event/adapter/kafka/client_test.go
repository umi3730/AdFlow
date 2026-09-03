package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/zhanghaiyang/adflow/internal/event/domain"
)

func TestDecodeVersionedEvent(t *testing.T) {
	encoded, err := json.Marshal(Message{SchemaVersion: 1, Event: domain.Event{EventID: "event-1", RequestID: "request-1", Type: domain.Impression}})
	if err != nil {
		t.Fatal(err)
	}
	message, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if message.Event.EventID != "event-1" {
		t.Fatalf("event id = %q", message.Event.EventID)
	}
}

func TestDecodeRejectsUnknownSchema(t *testing.T) {
	encoded, _ := json.Marshal(Message{SchemaVersion: 2, Event: domain.Event{EventID: "event-1", RequestID: "request-1"}})
	if _, err := Decode(encoded); err == nil {
		t.Fatal("expected schema version error")
	}
}

func TestProcessPartitionStopsAtFailureAndKeepsContiguousCommit(t *testing.T) {
	consumer := &Consumer{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	records := make([]*kgo.Record, 0, 3)
	for index := 0; index < 3; index++ {
		value, err := json.Marshal(Message{SchemaVersion: SchemaVersion, Event: domain.Event{
			EventID: "event-" + string(rune('1'+index)), RequestID: "request-1", Type: domain.Impression,
		}})
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, &kgo.Record{Topic: "events", Partition: 2, Offset: int64(index), Value: value})
	}
	var handled []string
	injected := errors.New("injected processing failure")
	result := consumer.processPartition(context.Background(), kgo.FetchTopicPartition{
		Topic: "events", Partition: 2, HighWatermark: 3, Records: records,
	}, map[string]int64{"events:2": 3}, func(_ context.Context, event domain.Event) error {
		handled = append(handled, event.EventID)
		if event.EventID == "event-2" {
			return injected
		}
		return nil
	})
	if len(handled) != 2 || handled[0] != "event-1" || handled[1] != "event-2" {
		t.Fatalf("handled=%v", handled)
	}
	if result.commit == nil || result.commit.Offset != 0 {
		t.Fatalf("commit=%v", result.commit)
	}
	if result.retry == nil || result.retry.Offset != 1 {
		t.Fatalf("retry=%v", result.retry)
	}
}

func TestProcessPartitionBatchCommitsLastRecordAfterAtomicHandler(t *testing.T) {
	consumer := &Consumer{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	records := make([]*kgo.Record, 0, 3)
	for index := 0; index < 3; index++ {
		value, err := json.Marshal(Message{SchemaVersion: SchemaVersion, Event: domain.Event{
			EventID: "batch-event-" + string(rune('1'+index)), RequestID: "request-1", Type: domain.Impression,
		}})
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, &kgo.Record{Topic: "events", Partition: 1, Offset: int64(index + 10), Value: value})
	}
	var handled []domain.Event
	result := consumer.processPartitionBatch(context.Background(), kgo.FetchTopicPartition{
		Topic: "events", Partition: 1, HighWatermark: 13, Records: records,
	}, map[string]int64{"events:1": 13}, func(_ context.Context, events []domain.Event) error {
		handled = append(handled, events...)
		return nil
	})
	if len(handled) != 3 || handled[0].EventID != "batch-event-1" || handled[2].EventID != "batch-event-3" {
		t.Fatalf("handled=%v", handled)
	}
	if result.retry != nil || result.commit == nil || result.commit.Offset != 12 {
		t.Fatalf("result=%+v", result)
	}
}
