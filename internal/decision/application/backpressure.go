package application

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/zhanghaiyang/adflow/internal/decision/domain"
)

type BackpressureConfig struct {
	SettlementHigh, SettlementLow int64
	OutboxHigh, OutboxLow         int64
}

type BackpressureObserver interface {
	SetDecisionBacklog(int64, int64, string)
	ObserveDecisionBackpressure(string)
}

type backlogSnapshot struct {
	at          time.Time
	counts      domain.AsyncBacklog
	blocked     bool
	unavailable bool
}

// One monitor per API samples shared SQL queues; requests only read a snapshot.
// This is feedback control, not a strict distributed outstanding-work limit.
type BackpressureGate struct {
	reader   domain.BacklogReader
	cfg      BackpressureConfig
	observer BackpressureObserver
	snapshot atomic.Pointer[backlogSnapshot]
	now      func() time.Time
}

func NewBackpressureGate(reader domain.BacklogReader, cfg BackpressureConfig, observer BackpressureObserver) (*BackpressureGate, error) {
	if reader == nil || cfg.SettlementHigh <= 0 || cfg.SettlementLow < 0 || cfg.SettlementLow >= cfg.SettlementHigh || cfg.OutboxHigh <= 0 || cfg.OutboxLow < 0 || cfg.OutboxLow >= cfg.OutboxHigh {
		return nil, errors.New("backpressure requires a reader and 0 <= low < high queue thresholds")
	}
	return &BackpressureGate{reader: reader, cfg: cfg, observer: observer, now: time.Now}, nil
}

func (g *BackpressureGate) Check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s := g.snapshot.Load()
	result := "allowed"
	var err error
	if s == nil || s.unavailable || g.now().Sub(s.at) >= time.Second {
		result, err = "unavailable", domain.ErrBackpressureUnavailable
	} else if s.blocked {
		result, err = "blocked", domain.ErrBackpressure
	}
	if g.observer != nil {
		g.observer.ObserveDecisionBackpressure(result)
	}
	return err
}

// Run has a single owner; cancellation closes the gate even before staleness.
func (g *BackpressureGate) Run(ctx context.Context) {
	defer func() {
		g.snapshot.Store(&backlogSnapshot{unavailable: true})
		if g.observer != nil {
			g.observer.SetDecisionBacklog(0, 0, "unavailable")
		}
	}()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for ctx.Err() == nil {
		g.refresh(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (g *BackpressureGate) refresh(ctx context.Context) {
	started := g.now()
	queryCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	counts, err := g.reader.ReadDecisionBacklog(queryCtx, g.cfg.SettlementHigh, g.cfg.OutboxHigh)
	cancel()
	s := &backlogSnapshot{at: started, counts: counts, unavailable: err != nil}
	if old := g.snapshot.Load(); old != nil {
		s.blocked = old.blocked
	}
	state := "unavailable"
	if err == nil {
		if counts.Settlements >= g.cfg.SettlementHigh || counts.Outbox >= g.cfg.OutboxHigh {
			s.blocked = true
		} else if counts.Settlements <= g.cfg.SettlementLow && counts.Outbox <= g.cfg.OutboxLow {
			s.blocked = false
		}
		state = "open"
		if s.blocked {
			state = "blocked"
		}
	}
	g.snapshot.Store(s)
	if g.observer != nil {
		g.observer.SetDecisionBacklog(counts.Settlements, counts.Outbox, state)
	}
}
