package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zhanghaiyang/adflow/internal/event/domain"
)

type fakeOutbox struct {
	entries     []domain.OutboxEntry
	publishedID string
	failedID    string
	nextAttempt time.Time
}

func (o *fakeOutbox) Enqueue(context.Context, domain.Event) (bool, error) { return true, nil }
func (o *fakeOutbox) ClaimBatch(context.Context, int, time.Time, time.Duration) ([]domain.OutboxEntry, error) {
	return o.entries, nil
}
func (o *fakeOutbox) MarkPublished(_ context.Context, id string, _ time.Time) error {
	o.publishedID = id
	return nil
}
func (o *fakeOutbox) MarkFailed(_ context.Context, id, _ string, next time.Time) error {
	o.failedID = id
	o.nextAttempt = next
	return nil
}
func (o *fakeOutbox) MarkDeadLetter(_ context.Context, id, _ string, _ time.Time) error {
	o.failedID = id
	return nil
}
func (o *fakeOutbox) Stats(context.Context) (domain.OutboxStats, error) {
	return domain.OutboxStats{}, nil
}

type fakePublisher struct{ err error }

func (p fakePublisher) PublishEvent(context.Context, domain.Event) error { return p.err }

type fakeBatchOutbox struct {
	*fakeOutbox
	publishedIDs []string
}

func (o *fakeBatchOutbox) MarkPublishedBatch(_ context.Context, ids []string, _ time.Time) error {
	o.publishedIDs = append(o.publishedIDs, ids...)
	return nil
}

type fakeBatchPublisher struct {
	events []domain.Event
}

func (p *fakeBatchPublisher) PublishEvent(_ context.Context, event domain.Event) error {
	p.events = append(p.events, event)
	return nil
}

func (p *fakeBatchPublisher) PublishEvents(_ context.Context, events []domain.Event) error {
	p.events = append(p.events, events...)
	return nil
}

type fakeDeadLetters struct{ eventID string }

func (p *fakeDeadLetters) PublishDeadLetter(_ context.Context, event domain.Event, _ string, _ int, _ time.Time) error {
	p.eventID = event.EventID
	return nil
}

func TestOutboxRelayMarksPublishedAfterAck(t *testing.T) {
	outbox := &fakeOutbox{entries: []domain.OutboxEntry{{Event: domain.Event{EventID: "event-1"}}}}
	relay := NewOutboxRelay(outbox, fakePublisher{}, nil, nil)
	relay.now = func() time.Time { return time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC) }
	count, err := relay.RunOnce(context.Background())
	if err != nil || count != 1 || outbox.publishedID != "event-1" {
		t.Fatalf("count=%d published=%s err=%v", count, outbox.publishedID, err)
	}
}

func TestOutboxRelayPublishesAndMarksBatch(t *testing.T) {
	outbox := &fakeBatchOutbox{fakeOutbox: &fakeOutbox{entries: []domain.OutboxEntry{
		{Event: domain.Event{EventID: "event-1"}},
		{Event: domain.Event{EventID: "event-2"}},
	}}}
	publisher := &fakeBatchPublisher{}
	relay := NewOutboxRelay(outbox, publisher, nil, nil)
	count, err := relay.RunOnce(context.Background())
	if err != nil || count != 2 || len(publisher.events) != 2 || len(outbox.publishedIDs) != 2 {
		t.Fatalf("count=%d published=%v marked=%v err=%v", count, publisher.events, outbox.publishedIDs, err)
	}
}

func TestOutboxRelaySchedulesRetryAfterPublishFailure(t *testing.T) {
	now := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	outbox := &fakeOutbox{entries: []domain.OutboxEntry{{Event: domain.Event{EventID: "event-1"}, Attempts: 2}}}
	relay := NewOutboxRelay(outbox, fakePublisher{err: errors.New("broker unavailable")}, nil, nil)
	relay.now = func() time.Time { return now }
	count, err := relay.RunOnce(context.Background())
	if err != nil || count != 0 || outbox.failedID != "event-1" || !outbox.nextAttempt.Equal(now.Add(4*time.Second)) {
		t.Fatalf("count=%d failed=%s next=%v err=%v", count, outbox.failedID, outbox.nextAttempt, err)
	}
}

func TestOutboxRelayDeadLettersAfterMaximumAttempts(t *testing.T) {
	outbox := &fakeOutbox{entries: []domain.OutboxEntry{{Event: domain.Event{EventID: "event-1"}, Attempts: 7}}}
	deadLetters := &fakeDeadLetters{}
	relay := NewOutboxRelay(outbox, fakePublisher{err: errors.New("permanent failure")}, deadLetters, nil)
	count, err := relay.RunOnce(context.Background())
	if err != nil || count != 0 || deadLetters.eventID != "event-1" || outbox.failedID != "event-1" {
		t.Fatalf("count=%d dlq=%s marked=%s err=%v", count, deadLetters.eventID, outbox.failedID, err)
	}
}
