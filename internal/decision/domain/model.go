package domain

import (
	"strings"
	"time"
)

type Request struct {
	ProfileDigest string
	RequestID     string
	UserID        string
	SlotID        string
	Now           time.Time
}

type Result struct {
	RequestFingerprint string
	Pricing            Pricing
	RequestID          string
	UserID             string
	SlotID             string
	Matched            bool
	CampaignID         string
	CreativeID         string
	ReservationToken   string
	ExpiresAt          time.Time
	Reason             Reason
}

type Reason string

const (
	ReasonMatched               Reason = "matched"
	ReasonProfileNotFound       Reason = "profile_not_found"
	ReasonNoCandidate           Reason = "no_candidate"
	ReasonNoCreative            Reason = "no_creative"
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
	profile := Profile{UserID: strings.TrimSpace(userID), Tags: make(map[string]struct{}, len(tags)), Fields: make(map[string]string, len(fields))}
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
	Tag   string `json:"tag,omitempty"`
	Field string `json:"field,omitempty"`
	Op    string `json:"op,omitempty"`
	Value string `json:"value,omitempty"`
}

type TargetingRule struct {
	All  []Condition
	Any  []Condition
	None []Condition
}

type Candidate struct {
	AdvertiserID      string
	AdvertiserName    string
	BidFen            int64
	Version           uint32
	CampaignID        string
	CreativeIDs       []string
	Targeting         TargetingRule
	DailyBudgetFen    int64
	ImpressionCostFen int64
	FrequencyLimit    uint32
	StartAt           time.Time
	EndAt             time.Time
}

// Comparable immutable settlement summary, persisted with the decision.
type Pricing struct {
	Mode              string `json:"mode"`
	AdvertiserID      string `json:"advertiserId,omitempty"`
	AdvertiserName    string `json:"advertiserName,omitempty"`
	BidFen            int64  `json:"bidFen,omitempty"`
	PriceFen          int64  `json:"priceFen"`
	Version           uint32 `json:"version"`
	Advertisers       int    `json:"advertisers"`
	Rank              int    `json:"rank"`
	BudgetRejected    int    `json:"budgetRejected"`
	FrequencyRejected int    `json:"frequencyRejected"`
}
