package domain

import "time"

type Request struct {
	RequestID string
	UserID    string
	SlotID    string
	Now       time.Time
}

type Result struct {
	RequestID        string
	UserID           string
	SlotID           string
	Matched          bool
	CampaignID       string
	CreativeID       string
	ReservationToken string
	ExpiresAt        time.Time
	Reason           Reason
}

type Reason string

const (
	ReasonMatched               Reason = "matched"
	ReasonProfileNotFound       Reason = "profile_not_found"
	ReasonNoCandidate           Reason = "no_candidate"
	ReasonTargetingMiss         Reason = "targeting_miss"
	ReasonFrequencyCapped       Reason = "frequency_capped"
	ReasonBudgetExhausted       Reason = "budget_exhausted"
	ReasonDependencyUnavailable Reason = "dependency_unavailable"
)

type Profile struct {
	UserID string
	Tags   map[string]struct{}
	Fields map[string]string
}

func NewProfile(userID string, tags []string, fields map[string]string) Profile {
	profile := Profile{UserID: userID, Tags: make(map[string]struct{}, len(tags)), Fields: make(map[string]string, len(fields))}
	for _, tag := range tags {
		if tag != "" {
			profile.Tags[tag] = struct{}{}
		}
	}
	for key, value := range fields {
		profile.Fields[key] = value
	}
	return profile
}

type Condition struct {
	Tag   string
	Field string
	Op    string
	Value string
}

type TargetingRule struct {
	All  []Condition
	Any  []Condition
	None []Condition
}

type Candidate struct {
	CampaignID        string
	CreativeIDs       []string
	Targeting         TargetingRule
	DailyBudgetFen    int64
	ImpressionCostFen int64
	FrequencyLimit    uint32
	StartAt           time.Time
	EndAt             time.Time
}
