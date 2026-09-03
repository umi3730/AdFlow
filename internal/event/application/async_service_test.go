package application

import (
	"context"
	"testing"
	"time"

	decisionmemory "github.com/zhanghaiyang/adflow/internal/decision/adapter/memory"
	decisiondomain "github.com/zhanghaiyang/adflow/internal/decision/domain"
	eventmemory "github.com/zhanghaiyang/adflow/internal/event/adapter/memory"
	"github.com/zhanghaiyang/adflow/internal/event/domain"
)

type recordingOutbox struct {
	events []domain.Event
}

func (p *recordingOutbox) Enqueue(_ context.Context, event domain.Event) (bool, error) {
	p.events = append(p.events, event)
	return true, nil
}

func TestAsyncServiceValidatesThenPublishes(t *testing.T) {
	ctx := context.Background()
	decisions := decisionmemory.NewRuntime()
	if err := decisions.SaveDecision(ctx, decisiondomain.Result{
		RequestID: "request-1", Matched: true, CampaignID: "campaign-1", CreativeID: "creative-1",
		ReservationToken: "request-1", ExpiresAt: time.Now().Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	processor := NewService(eventmemory.NewStore(), decisions, decisions)
	outbox := &recordingOutbox{}
	service := NewAsyncService(processor, decisions, outbox)
	created, err := service.Record(ctx, domain.Event{
		EventID: "event-1", RequestID: "request-1", CampaignID: "campaign-1", CreativeID: "creative-1", Type: domain.Impression,
	})
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if len(outbox.events) != 1 || outbox.events[0].OccurredAt.IsZero() {
		t.Fatalf("unexpected enqueued events: %+v", outbox.events)
	}
}
