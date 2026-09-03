package domain

import (
	"context"
	"time"
)

type CandidateProvider interface {
	ActiveCandidates(context.Context, string, time.Time) ([]Candidate, error)
}

type ProfileStore interface {
	FindProfile(context.Context, string) (Profile, error)
	PutProfile(context.Context, Profile) error
}

type FrequencyGate interface {
	ReserveFrequency(context.Context, string, string, string, uint32, time.Time, time.Duration) (string, bool, error)
	ReleaseFrequency(context.Context, string) error
	ConfirmFrequency(context.Context, string, time.Time) error
}

type BudgetGate interface {
	ReserveBudget(context.Context, string, int64, int64, string, time.Time, time.Duration) (string, bool, error)
	ReleaseBudget(context.Context, string) error
	ConfirmBudget(context.Context, string, time.Time) error
}

type DecisionStore interface {
	FindDecision(context.Context, string) (Result, bool, error)
	SaveDecision(context.Context, Result) error
}
