package application

import (
	"context"
	"errors"
	decisiondomain "github.com/zhanghaiyang/adflow/internal/decision/domain"
	"github.com/zhanghaiyang/adflow/internal/event/domain"
	"log/slog"
	"time"
)

type SettlementWorker struct {
	queue   domain.SettlementQueue
	settler decisiondomain.ImpressionSettler
	logger  *slog.Logger
}

func NewSettlementWorker(queue domain.SettlementQueue, settler decisiondomain.ImpressionSettler, logger *slog.Logger) *SettlementWorker {
	if logger == nil {
		logger = slog.Default()
	}
	return &SettlementWorker{queue: queue, settler: settler, logger: logger}
}

func (w *SettlementWorker) Run(ctx context.Context) error {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return nil
		}
		completed, err := w.RunOnce(ctx)
		if err != nil && ctx.Err() == nil {
			w.logger.Error("settlement worker iteration failed", "error", err)
		}
		// Drain healthy work immediately; polling only backs off when idle or
		// failing, rather than imposing a 10-events-per-tick throughput ceiling.
		if err == nil && completed > 0 {
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (w *SettlementWorker) RunOnce(ctx context.Context) (completed int, err error) {
	// Four bounded attempts fit within the lease; the lease is shorter than a
	// fresh 30s reservation even if cleanup cannot run after a process crash.
	claimCtx, claimCancel := context.WithTimeout(ctx, 2*time.Second)
	entries, err := w.queue.ClaimSettlements(claimCtx, 4, 15*time.Second)
	claimCancel()
	if err != nil {
		return 0, err
	}
	unfinished := entries
	defer func() {
		if len(unfinished) == 0 {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		err = errors.Join(err, w.queue.ReleaseSettlements(cleanupCtx, unfinished))
	}()
	for i, entry := range entries {
		if err = ctx.Err(); err != nil {
			return completed, err
		}
		var settled bool
		settled, err = w.settleEntry(ctx, entry)
		if err != nil {
			return completed, err
		}
		unfinished = entries[i+1:]
		if settled {
			completed++
		}
	}
	return completed, nil
}

func (w *SettlementWorker) settleEntry(ctx context.Context, entry domain.SettlementEntry) (bool, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	err := w.settler.SettleImpression(attemptCtx, entry.Settlement)
	if err == nil {
		// A retry after Redis success uses the existing settlement receipt.
		err = w.queue.CompleteSettlement(attemptCtx, entry)
		return err == nil, err
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	review := errors.Is(err, decisiondomain.ErrSettlementUnavailable) || entry.Attempts+1 >= 8
	failureCtx, failureCancel := context.WithTimeout(ctx, time.Second)
	defer failureCancel()
	if failureErr := w.queue.FailSettlement(failureCtx, entry, err.Error(), retryDelay(entry.Attempts+1), review); failureErr != nil {
		return false, failureErr
	}
	if review {
		w.logger.Warn("impression requires settlement reconciliation", "request_id", entry.Settlement.Decision.RequestID, "error", err)
	}
	return false, nil
}

// Guard even broker replay: an event without durable settlement proof must not
// reach the metrics writer, including events published by the legacy relay.
func RecordSettledBatch(proof domain.SettlementProof, processor *Service) func(context.Context, []domain.Event) error {
	return func(ctx context.Context, events []domain.Event) error {
		if err := proof.VerifySettledEvents(ctx, events); err != nil {
			return err
		}
		return processor.RecordBatch(ctx, events)
	}
}
