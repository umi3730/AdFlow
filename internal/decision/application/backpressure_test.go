package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zhanghaiyang/adflow/internal/decision/adapter/memory"
	"github.com/zhanghaiyang/adflow/internal/decision/domain"
)

type backlogReaderFunc func(context.Context, int64, int64) (domain.AsyncBacklog, error)

func (f backlogReaderFunc) ReadDecisionBacklog(ctx context.Context, a, b int64) (domain.AsyncBacklog, error) {
	return f(ctx, a, b)
}

func TestBackpressureHysteresisUnavailableAndStaleness(t *testing.T) {
	now := time.Now()
	counts := domain.AsyncBacklog{}
	var readErr error
	gate, err := NewBackpressureGate(backlogReaderFunc(func(ctx context.Context, a, b int64) (domain.AsyncBacklog, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 201*time.Millisecond || a != 100 || b != 600 {
			t.Fatal("unbounded query or wrong caps")
		}
		return counts, readErr
	}), BackpressureConfig{100, 40, 600, 240}, nil)
	if err != nil {
		t.Fatal(err)
	}
	gate.now = func() time.Time { return now }
	assertGate := func(want error) {
		t.Helper()
		if err := gate.Check(t.Context()); !errors.Is(err, want) {
			t.Fatalf("got %v want %v", err, want)
		}
	}
	assertGate(domain.ErrBackpressureUnavailable)
	gate.refresh(t.Context())
	assertGate(nil)
	counts.Settlements = 100
	gate.refresh(t.Context())
	assertGate(domain.ErrBackpressure)
	counts = domain.AsyncBacklog{Settlements: 41, Outbox: 240}
	gate.refresh(t.Context())
	assertGate(domain.ErrBackpressure)
	readErr = errors.New("database unavailable")
	gate.refresh(t.Context())
	assertGate(domain.ErrBackpressureUnavailable)
	readErr = nil
	gate.refresh(t.Context())
	assertGate(domain.ErrBackpressure) // transient error must not reset hysteresis
	counts.Settlements = 40
	gate.refresh(t.Context())
	assertGate(nil)
	counts.Outbox = 600
	gate.refresh(t.Context())
	assertGate(domain.ErrBackpressure)
	counts.Outbox = 241
	gate.refresh(t.Context())
	assertGate(domain.ErrBackpressure)
	counts.Outbox = 240
	gate.refresh(t.Context())
	assertGate(nil)
	now = now.Add(time.Second)
	assertGate(domain.ErrBackpressureUnavailable)
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := gate.Check(canceled); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestBackpressureMonitorStopsClosed(t *testing.T) {
	entered := make(chan struct{})
	gate, _ := NewBackpressureGate(backlogReaderFunc(func(ctx context.Context, _, _ int64) (domain.AsyncBacklog, error) {
		close(entered)
		<-ctx.Done()
		return domain.AsyncBacklog{}, ctx.Err()
	}), BackpressureConfig{100, 40, 600, 240}, nil)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { gate.Run(ctx); close(done) }()
	<-entered
	for i := 0; i < 100; i++ {
		_ = gate.Check(t.Context())
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("monitor did not stop")
	}
	if err := gate.Check(t.Context()); !errors.Is(err, domain.ErrBackpressureUnavailable) {
		t.Fatal(err)
	}
}

type newDecisionGateFunc func(context.Context) error

func (f newDecisionGateFunc) Check(ctx context.Context) error { return f(ctx) }

func TestBackpressureRejectsBeforeWritesAndPreservesReplay(t *testing.T) {
	runtime := memory.NewRuntime()
	service := NewService(nil, nil, nil, nil, runtime)
	request := domain.Request{RequestID: "new", UserID: "user", SlotID: "slot"}
	service.SetNewDecisionGate(newDecisionGateFunc(func(context.Context) error { return domain.ErrBackpressure }))
	if _, err := service.Decide(t.Context(), request); !errors.Is(err, domain.ErrBackpressure) {
		t.Fatal(err)
	}
	if _, found, err := runtime.FindDecision(t.Context(), request.RequestID); found || err != nil {
		t.Fatal("rejection persisted a decision")
	}
	// A failed admission must not retain an execution lease either.
	if err := runtime.AcquireDecision(t.Context(), request, "check", time.Minute); err != nil {
		t.Fatal(err)
	}
	_ = runtime.ReleaseDecision(t.Context(), request.RequestID, "check")
	existing := domain.Result{RequestID: request.RequestID, UserID: request.UserID, SlotID: request.SlotID, Matched: false, Reason: domain.ReasonNoCandidate}
	if err := runtime.SaveDecision(t.Context(), existing); err != nil {
		t.Fatal(err)
	}
	if got, err := service.Decide(t.Context(), request); err != nil || got != existing {
		t.Fatalf("replay: %v %v", got, err)
	}
	request.UserID = "different"
	if _, err := service.Decide(t.Context(), request); !errors.Is(err, domain.ErrRequestConflict) {
		t.Fatal("conflict hidden by pressure", err)
	}
}
