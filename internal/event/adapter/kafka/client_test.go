package kafka

import (
	"encoding/json"
	"testing"

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
