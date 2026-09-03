package domain

import (
	"strconv"
	"strings"
)

type Evaluator struct{}

func (Evaluator) Match(profile Profile, rule TargetingRule) bool {
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

func matchCondition(profile Profile, condition Condition) bool {
	if condition.Tag != "" {
		_, exists := profile.Tags[condition.Tag]
		return exists
	}
	actual, exists := profile.Fields[condition.Field]
	if !exists {
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
	case "gte", "lte":
		actualNumber, actualErr := strconv.ParseFloat(actual, 64)
		expectedNumber, expectedErr := strconv.ParseFloat(condition.Value, 64)
		if actualErr != nil || expectedErr != nil {
			return false
		}
		if condition.Op == "gte" {
			return actualNumber >= expectedNumber
		}
		return actualNumber <= expectedNumber
	default:
		return false
	}
}
