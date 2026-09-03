package application

import (
	"context"
	"errors"
	"testing"
	"time"

	decisionmemory "github.com/zhanghaiyang/adflow/internal/decision/adapter/memory"
	decisiondomain "github.com/zhanghaiyang/adflow/internal/decision/domain"
	eventmemory "github.com/zhanghaiyang/adflow/internal/event/adapter/memory"
	"github.com/zhanghaiyang/adflow/internal/event/domain"
)

type recordingOutbox struct {
	events         []domain.Event
	createdResults []bool
}

type recordingConfirmer struct {
	frequency int
	budget    int
	budgetErr error
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
	confirmer := &recordingConfirmer{}
	service := NewAsyncService(processor, decisions, outbox, confirmer)
	created, err := service.Record(ctx, domain.Event{
		EventID: "event-1", RequestID: "request-1", CampaignID: "campaign-1", CreativeID: "creative-1", Type: domain.Impression,
	})
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if len(outbox.events) != 1 || outbox.events[0].OccurredAt.IsZero() {
		t.Fatalf("unexpected enqueued events: %+v", outbox.events)
	}
	if confirmer.frequency != 1 || confirmer.budget != 1 {
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
	service := NewAsyncService(NewService(eventmemory.NewStore(), decisions, confirmer), decisions, outbox, confirmer)
	_, err := service.Record(ctx, domain.Event{
		EventID: "expired-event", RequestID: "expired-request", CampaignID: "campaign-1", CreativeID: "creative-1",
		Type: domain.Impression, OccurredAt: now,
	})
	if err != domain.ErrDecisionExpired || len(outbox.events) != 0 {
		t.Fatalf("err=%v events=%v", err, outbox.events)
	}
}

func TestAsyncServiceDuplicateRetriesReservationSettlement(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	decisions := fixedDecisionFinder{result: decisiondomain.Result{
		RequestID: "request-1", Matched: true, CampaignID: "campaign-1", CreativeID: "creative-1",
		ReservationToken: "request-1", ExpiresAt: now.Add(time.Minute),
	}}
	outbox := &recordingOutbox{createdResults: []bool{true, false}}
	confirmer := &recordingConfirmer{budgetErr: errors.New("temporary Redis failure")}
	service := NewAsyncService(NewService(eventmemory.NewStore(), decisions, confirmer), decisions, outbox, confirmer)
	event := domain.Event{
		EventID: "event-1", RequestID: "request-1", CampaignID: "campaign-1", CreativeID: "creative-1",
		Type: domain.Impression, OccurredAt: now,
	}
	if _, err := service.Record(ctx, event); err == nil {
		t.Fatal("expected the first reservation settlement to fail")
	}
	created, err := service.Record(ctx, event)
	if err != nil || created {
		t.Fatalf("duplicate recovery created=%v err=%v", created, err)
	}
	if confirmer.budget != 2 || confirmer.frequency != 1 {
		t.Fatalf("confirmation counts after recovery: %+v", confirmer)
	}
}
