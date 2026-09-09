package application

import (
	"context"
	"time"

	"github.com/umi3730/adflow/internal/event/domain"
)

type OutboxRelay struct {
	outbox      domain.Outbox
	publisher   domain.Publisher
	deadLetters domain.DeadLetterPublisher
	observer    interface {
		SetOutboxDepth(domain.OutboxStats)
		ObserveOutboxResult(string)
	}
	batchSize    int
	maxAttempts  int
	pollInterval time.Duration
	lease        time.Duration
	now          func() time.Time
}

func NewOutboxRelay(outbox domain.Outbox, publisher domain.Publisher, deadLetters domain.DeadLetterPublisher, observer interface {
	SetOutboxDepth(domain.OutboxStats)
	ObserveOutboxResult(string)
}) *OutboxRelay {
	return &OutboxRelay{
		outbox: outbox, publisher: publisher, deadLetters: deadLetters, observer: observer, batchSize: 100, maxAttempts: 8,
		pollInterval: 500 * time.Millisecond, lease: 30 * time.Second, now: time.Now,
	}
}

func (r *OutboxRelay) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()
	for {
		if _, err := r.RunOnce(ctx); err != nil && ctx.Err() != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (r *OutboxRelay) RunOnce(ctx context.Context) (int, error) {
	now := r.now().UTC()
	entries, err := r.outbox.ClaimBatch(ctx, r.batchSize, now, r.lease)
	if err != nil {
		return 0, err
	}
	if batchPublisher, publishOK := r.publisher.(domain.BatchPublisher); publishOK {
		if batchMarker, markOK := r.outbox.(domain.PublishedBatchMarker); markOK && len(entries) > 0 {
			return r.publishBatch(ctx, entries, batchPublisher, batchMarker, now)
		}
	}
	published := 0
	for _, entry := range entries {
		if err := r.publisher.PublishEvent(ctx, entry.Event); err != nil {
			attempts := entry.Attempts + 1
			if attempts >= r.maxAttempts && r.deadLetters != nil {
				if deadErr := r.deadLetters.PublishDeadLetter(ctx, entry.Event, err.Error(), attempts, now); deadErr == nil {
					if markErr := r.outbox.MarkDeadLetter(ctx, entry.Event.EventID, err.Error(), now); markErr != nil {
						return published, markErr
					}
					if r.observer != nil {
						r.observer.ObserveOutboxResult("dead_lettered")
					}
					continue
				}
			}
			nextAttempt := now.Add(retryDelay(attempts))
			if markErr := r.outbox.MarkFailed(ctx, entry.Event.EventID, err.Error(), nextAttempt); markErr != nil {
				return published, markErr
			}
			if r.observer != nil {
				r.observer.ObserveOutboxResult("retry")
			}
			continue
		}
		if err := r.outbox.MarkPublished(ctx, entry.Event.EventID, now); err != nil {
			return published, err
		}
		published++
		if r.observer != nil {
			r.observer.ObserveOutboxResult("published")
		}
	}
	if r.observer != nil {
		stats, statsErr := r.outbox.Stats(ctx)
		if statsErr == nil {
			r.observer.SetOutboxDepth(stats)
		}
	}
	return published, nil
}

func (r *OutboxRelay) publishBatch(ctx context.Context, entries []domain.OutboxEntry, publisher domain.BatchPublisher, marker domain.PublishedBatchMarker, now time.Time) (int, error) {
	events := make([]domain.Event, len(entries))
	eventIDs := make([]string, len(entries))
	for index, entry := range entries {
		events[index] = entry.Event
		eventIDs[index] = entry.Event.EventID
	}
	if err := publisher.PublishEvents(ctx, events); err != nil {
		for _, entry := range entries {
			attempts := entry.Attempts + 1
			if attempts >= r.maxAttempts && r.deadLetters != nil {
				if deadErr := r.deadLetters.PublishDeadLetter(ctx, entry.Event, err.Error(), attempts, now); deadErr == nil {
					if markErr := r.outbox.MarkDeadLetter(ctx, entry.Event.EventID, err.Error(), now); markErr != nil {
						return 0, markErr
					}
					if r.observer != nil {
						r.observer.ObserveOutboxResult("dead_lettered")
					}
					continue
				}
			}
			if markErr := r.outbox.MarkFailed(ctx, entry.Event.EventID, err.Error(), now.Add(retryDelay(attempts))); markErr != nil {
				return 0, markErr
			}
			if r.observer != nil {
				r.observer.ObserveOutboxResult("retry")
			}
		}
		return 0, nil
	}
	if err := marker.MarkPublishedBatch(ctx, eventIDs, now); err != nil {
		return 0, err
	}
	if r.observer != nil {
		for range entries {
			r.observer.ObserveOutboxResult("published")
		}
		if stats, err := r.outbox.Stats(ctx); err == nil {
			r.observer.SetOutboxDepth(stats)
		}
	}
	return len(entries), nil
}

func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := time.Second << min(attempt-1, 6)
	if delay > time.Minute {
		return time.Minute
	}
	return delay
}
