package application

import (
	"context"
	"errors"
	"testing"
	"time"

	decisionmemory "github.com/umi3730/adflow/internal/decision/adapter/memory"
	decisiondomain "github.com/umi3730/adflow/internal/decision/domain"
	eventmemory "github.com/umi3730/adflow/internal/event/adapter/memory"
	"github.com/umi3730/adflow/internal/event/domain"
)

type recordingOutbox struct {
	events         []domain.Event
	createdResults []bool
}

func (p *recordingOutbox) FindAcceptedEvent(_ context.Context, id string) (domain.Event, bool, error) {
	for _, event := range p.events {
		if event.EventID == id {
			return event, true, nil
		}
	}
	return domain.Event{}, false, nil
}

func (p *recordingOutbox) EnqueueForSettlement(ctx context.Context, event domain.Event, _ decisiondomain.Result, _ time.Time) (bool, error) {
	return p.Enqueue(ctx, event)
}

type recordingConfirmer struct {
	frequency int
	budget    int
	budgetErr error
}

func (c *recordingConfirmer) SettleImpression(ctx context.Context, settlement decisiondomain.Settlement) error {
	if err := c.ConfirmBudget(ctx, settlement.Decision.ReservationToken, time.Now()); err != nil {
		return err
	}
	return c.ConfirmFrequency(ctx, settlement.Decision.ReservationToken, time.Now())
}

func (p *recordingOutbox) FindImpressionReceipt(_ context.Context, requestID string) (domain.ImpressionReceipt, bool, error) {
	for _, event := range p.events {
		if event.RequestID == requestID && event.Type == domain.Impression {
			return domain.ImpressionReceipt{Event: event, AcceptedAt: event.OccurredAt}, true, nil
		}
	}
	return domain.ImpressionReceipt{}, false, nil
}

type fixedDecisionFinder struct{ result decisiondomain.Result }

func (f fixedDecisionFinder) FindDecision(context.Context, string) (decisiondomain.Result, bool, error) {
	return f.result, true, nil
}

func (c *recordingConfirmer) ConfirmFrequency(context.Context, string, time.Time) error {
	c.frequency++
	return nil
}

func (c *recordingConfirmer) ConfirmBudget(context.Context, string, time.Time) error {
	c.budget++
	if c.budgetErr != nil {
		err := c.budgetErr
		c.budgetErr = nil
		return err
	}
	return nil
}

func (p *recordingOutbox) Enqueue(_ context.Context, event domain.Event) (bool, error) {
	p.events = append(p.events, event)
	if len(p.createdResults) > 0 {
		created := p.createdResults[0]
		p.createdResults = p.createdResults[1:]
		return created, nil
	}
	return true, nil
}

func TestAsyncServiceDurablyQueuesWithoutInlineSettlement(t *testing.T) {
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
	confirmer := &recordingConfirmer{}
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
	if confirmer.frequency != 0 || confirmer.budget != 0 {
		t.Fatalf("confirmation counts: %+v", confirmer)
	}
}

func TestAsyncServiceRejectsExpiredDecision(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	decisions := fixedDecisionFinder{result: decisiondomain.Result{
		RequestID: "expired-request", Matched: true, CampaignID: "campaign-1", CreativeID: "creative-1",
		ReservationToken: "expired-request", ExpiresAt: now.Add(-time.Second),
	}}
	outbox := &recordingOutbox{}
	confirmer := &recordingConfirmer{}
	service := NewAsyncService(NewService(eventmemory.NewStore(), decisions, confirmer), decisions, outbox)
	_, err := service.Record(ctx, domain.Event{
		EventID: "expired-event", RequestID: "expired-request", CampaignID: "campaign-1", CreativeID: "creative-1",
		Type: domain.Impression, OccurredAt: now,
	})
	if err != domain.ErrDecisionExpired || len(outbox.events) != 0 {
		t.Fatalf("err=%v events=%v", err, outbox.events)
	}
}

func TestAsyncServiceDuplicateAfterExpiryKeepsOriginalEvent(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	decisions := fixedDecisionFinder{result: decisiondomain.Result{
		RequestID: "request-1", Matched: true, CampaignID: "campaign-1", CreativeID: "creative-1",
		ReservationToken: "request-1", ExpiresAt: now.Add(time.Minute),
	}}
	outbox := &recordingOutbox{createdResults: []bool{true, false}}
	confirmer := &recordingConfirmer{budgetErr: errors.New("temporary Redis failure")}
	service := NewAsyncService(NewService(eventmemory.NewStore(), decisions, confirmer), decisions, outbox)
	event := domain.Event{
		EventID: "event-1", RequestID: "request-1", CampaignID: "campaign-1", CreativeID: "creative-1",
		Type: domain.Impression, OccurredAt: now,
	}
	if _, err := service.Record(ctx, event); err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now.Add(time.Hour) }
	created, err := service.Record(ctx, event)
	if err != nil || created {
		t.Fatalf("duplicate recovery created=%v err=%v", created, err)
	}
	if confirmer.budget != 0 || confirmer.frequency != 0 || len(outbox.events) != 1 {
		t.Fatalf("confirmation counts after recovery: %+v", confirmer)
	}
	event.CampaignID = "different"
	if _, err := service.Record(ctx, event); !errors.Is(err, domain.ErrEventConflict) {
		t.Fatalf("changed payload: %v", err)
	}
}
