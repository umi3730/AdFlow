package domain

import (
	"strings"
	"time"
)

type Name string

func NewName(value string) (Name, error) {
	value = strings.TrimSpace(value)
	if length := len([]rune(value)); length < 2 || length > 128 {
		return "", ErrInvalidName
	}
	return Name(value), nil
}

type SlotID string

func NewSlotID(value string) (SlotID, error) {
	value = strings.TrimSpace(value)
	if length := len([]rune(value)); length < 2 || length > 64 {
		return "", ErrInvalidSlotID
	}
	return SlotID(value), nil
}

type DeliveryPeriod struct {
	start time.Time
	end   time.Time
}

func NewDeliveryPeriod(start, end time.Time) (DeliveryPeriod, error) {
	if start.IsZero() || end.IsZero() || !end.After(start) {
		return DeliveryPeriod{}, ErrInvalidPeriod
	}
	return DeliveryPeriod{start: start.UTC(), end: end.UTC()}, nil
}

func (p DeliveryPeriod) Start() time.Time { return p.start }
func (p DeliveryPeriod) End() time.Time   { return p.end }

type Money struct {
	amount   int64
	currency string
}

func NewCNYFen(amount int64) (Money, error) {
	if amount <= 0 {
		return Money{}, ErrInvalidBudget
	}
	return Money{amount: amount, currency: "CNY"}, nil
}

func newImpressionCost(amount int64) (Money, error) {
	if amount <= 0 {
		return Money{}, ErrInvalidCost
	}
	return Money{amount: amount, currency: "CNY"}, nil
}

func (m Money) Amount() int64    { return m.amount }
func (m Money) Currency() string { return m.currency }

type Condition struct {
	Tag   string `json:"tag,omitempty"`
	Field string `json:"field,omitempty"`
	Op    string `json:"op,omitempty"`
	Value string `json:"value,omitempty"`
}

type TargetingRule struct {
	All  []Condition `json:"all,omitempty"`
	Any  []Condition `json:"any,omitempty"`
	None []Condition `json:"none,omitempty"`
}

func NewTargetingRule(all, anyOf, none []Condition) (TargetingRule, error) {
	rule := TargetingRule{
		All:  append([]Condition(nil), all...),
		Any:  append([]Condition(nil), anyOf...),
		None: append([]Condition(nil), none...),
	}
	if len(rule.All)+len(rule.Any)+len(rule.None) == 0 || len(rule.All)+len(rule.Any)+len(rule.None) > 50 {
		return TargetingRule{}, ErrInvalidTargeting
	}
	for _, group := range [][]Condition{rule.All, rule.Any, rule.None} {
		for _, condition := range group {
			hasTag := strings.TrimSpace(condition.Tag) != ""
			hasField := strings.TrimSpace(condition.Field) != "" && strings.TrimSpace(condition.Op) != "" && strings.TrimSpace(condition.Value) != ""
			if hasTag == hasField {
				return TargetingRule{}, ErrInvalidTargeting
			}
			if hasField && condition.Op != "eq" && condition.Op != "in" && condition.Op != "gte" && condition.Op != "lte" {
				return TargetingRule{}, ErrInvalidTargeting
			}
		}
	}
	return rule, nil
}

func (r TargetingRule) Clone() TargetingRule {
	return TargetingRule{
		All:  append([]Condition(nil), r.All...),
		Any:  append([]Condition(nil), r.Any...),
		None: append([]Condition(nil), r.None...),
	}
}
