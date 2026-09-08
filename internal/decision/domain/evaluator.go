package domain

import (
	"strings"

	"github.com/umi3730/adflow/internal/profile/schema"
)

type Evaluator struct{}

type ConditionFailure struct {
	Group     string    `json:"group"`
	Code      string    `json:"code"`
	Condition Condition `json:"condition"`
	Actual    string    `json:"actual,omitempty"`
	Present   bool      `json:"present"`
}

// Explain uses the same predicates as Match, but only runs on the opt-in
// diagnostic endpoint. It never reserves budget/frequency or writes a decision.
func (e Evaluator) Explain(profile Profile, rule TargetingRule) []ConditionFailure {
	if invalid := invalidFieldConditions(rule); len(invalid) > 0 {
		return invalid
	}
	failures := make([]ConditionFailure, 0)
	for _, condition := range rule.All {
		if !matchCondition(profile, condition) {
			failures = append(failures, explainFailure(profile, "all", condition, false))
		}
	}
	anyMatched := false
	for _, condition := range rule.Any {
		if matchCondition(profile, condition) {
			anyMatched = true
			break
		}
	}
	if len(rule.Any) > 0 && !anyMatched {
		for _, condition := range rule.Any {
			failures = append(failures, explainFailure(profile, "any", condition, false))
		}
	}
	for _, condition := range rule.None {
		if matchCondition(profile, condition) {
			failures = append(failures, explainFailure(profile, "none", condition, true))
		}
	}
	return failures
}

func explainFailure(profile Profile, group string, condition Condition, excluded bool) ConditionFailure {
	failure := ConditionFailure{Group: group, Condition: condition}
	if condition.Tag != "" {
		_, failure.Present = profile.Tags[condition.Tag]
		failure.Code = "missing_tag"
	} else {
		failure.Actual, failure.Present = profile.Fields[condition.Field]
		failure.Code = "value_mismatch"
		if !failure.Present {
			failure.Code = "missing_field"
		}
	}
	if excluded {
		failure.Code = "excluded_condition"
	}
	return failure
}

func (Evaluator) Match(profile Profile, rule TargetingRule) bool {
	if len(invalidFieldConditions(rule)) > 0 {
		return false
	}
	for _, condition := range rule.All {
		if !matchCondition(profile, condition) {
			return false
		}
	}
	if len(rule.Any) > 0 {
		matched := false
		for _, condition := range rule.Any {
			if matchCondition(profile, condition) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	for _, condition := range rule.None {
		if matchCondition(profile, condition) {
			return false
		}
	}
	return true
}

func invalidFieldConditions(rule TargetingRule) []ConditionFailure {
	var failures []ConditionFailure
	for _, group := range []struct {
		name string
		rows []Condition
	}{{"all", rule.All}, {"any", rule.Any}, {"none", rule.None}} {
		for _, condition := range group.rows {
			if condition.Tag == "" && schema.ValidateCondition(condition.Field, condition.Op, condition.Value) != nil {
				failures = append(failures, ConditionFailure{Group: group.name, Code: "invalid_condition", Condition: condition})
			}
		}
	}
	return failures
}

func matchCondition(profile Profile, condition Condition) bool {
	if condition.Tag != "" {
		_, exists := profile.Tags[condition.Tag]
		return exists
	}
	actual, exists := profile.Fields[condition.Field]
	if !exists {
		return false
	}
	// Text fields never gain numeric semantics merely because both values parse.
	if !schema.IsNumeric(condition.Field) && (condition.Op == "gte" || condition.Op == "lte") {
		return false
	}
	if schema.IsNumeric(condition.Field) {
		if schema.ValidateCondition(condition.Field, condition.Op, condition.Value) != nil {
			return false
		}
		actualNumber, valid := schema.NumericValue(condition.Field, actual)
		if !valid {
			return false
		}
		values := []string{condition.Value}
		if condition.Op == "in" {
			values = strings.Split(condition.Value, ",")
		}
		for _, value := range values {
			expected, ok := schema.NumericValue(condition.Field, value)
			if !ok {
				return false
			}
			switch condition.Op {
			case "eq", "in":
				if actualNumber == expected {
					return true
				}
			case "gte":
				return actualNumber >= expected
			case "lte":
				return actualNumber <= expected
			}
		}
		return false
	}
	switch condition.Op {
	case "eq":
		return actual == condition.Value
	case "in":
		for _, expected := range strings.Split(condition.Value, ",") {
			if actual == strings.TrimSpace(expected) {
				return true
			}
		}
		return false
	default:
		return false
	}
}
