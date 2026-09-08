package domain

import "context"

// Counts are capped at the reader's requested limits, not exact dashboard totals.
type AsyncBacklog struct {
	Settlements int64
	Outbox      int64
}

type BacklogReader interface {
	ReadDecisionBacklog(context.Context, int64, int64) (AsyncBacklog, error)
}

type NewDecisionGate interface {
	Check(context.Context) error
}
