package application

import (
	"errors"
	decisiondomain "github.com/zhanghaiyang/adflow/internal/decision/domain"
	eventmemory "github.com/zhanghaiyang/adflow/internal/event/adapter/memory"
	"github.com/zhanghaiyang/adflow/internal/event/domain"
	"testing"
	"time"
)

func TestRetainedDecisionCannotAuthorizeBackdatedLateExposure(t *testing.T) {
	now := time.Now().UTC()
	expiry := now.Add(-time.Second)
	finder := fixedDecisionFinder{result: decisiondomain.Result{RequestID: "retained", CampaignID: "campaign", CreativeID: "creative", Matched: true, ExpiresAt: expiry, ReservationToken: "token"}}
	event := domain.Event{EventID: "late", RequestID: "retained", CampaignID: "campaign", CreativeID: "creative", Type: domain.Impression, OccurredAt: expiry.Add(-time.Second)}
	confirmer := &recordingConfirmer{}
	syncService := NewService(eventmemory.NewStore(), finder, confirmer)
	if _, err := syncService.Record(t.Context(), event); !errors.Is(err, domain.ErrDecisionExpired) {
		t.Fatal(err)
	}
	outbox := &recordingOutbox{}
	asyncService := NewAsyncService(syncService, finder, outbox)
	if _, err := asyncService.Record(t.Context(), event); !errors.Is(err, domain.ErrDecisionExpired) || len(outbox.events) != 0 {
		t.Fatalf("err=%v queued=%d", err, len(outbox.events))
	}
	consumer := NewService(eventmemory.NewStore(), finder, nil)
	if _, err := consumer.Record(t.Context(), event); err != nil {
		t.Fatalf("previously accepted durable event rejected: %v", err)
	}
	if confirmer.budget != 0 || confirmer.frequency != 0 {
		t.Fatal("late ingress settled reservations")
	}
}
