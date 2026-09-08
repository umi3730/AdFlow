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

// ProfileCatalog reads saved profiles for administration, independently of the
// hot-path lookup/cache interface.
type ProfileCatalog interface {
	ListProfiles(context.Context, ProfileFilter) (ProfilePage, error)
}

type ProfileDeleter interface {
	DeleteProfile(context.Context, string) error
}

type ProfileFilter struct {
	Query  string
	Tag    string
	Device string
	Limit  int
	Offset int
}

type ProfilePage struct {
	Items []Profile
	Total int
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
	AcquireDecision(context.Context, Request, string, time.Duration) error
	CommitDecision(context.Context, Result, string) error
	ReleaseDecision(context.Context, string, string) error
}

// Inspection must not reserve/release resources or expire process state.
type DecisionInspector interface {
	PeekDecision(context.Context, string) (Result, bool, error)
}
