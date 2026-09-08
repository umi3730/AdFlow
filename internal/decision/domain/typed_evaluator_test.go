package domain

import "testing"

func TestTypedFieldComparisons(t *testing.T) {
	profile := NewProfile("user", nil, map[string]string{"score": "88", "country": "156", "device": "android"})
	for _, tc := range []struct {
		condition Condition
		want      bool
	}{
		{Condition{Field: "country", Op: "gte", Value: "1"}, false},
		{Condition{Field: "country", Op: "eq", Value: "156"}, true},
		{Condition{Field: "score", Op: "eq", Value: "88.0"}, true},
		{Condition{Field: "score", Op: "in", Value: "87,88.0"}, true},
		{Condition{Field: "score", Op: "in", Value: "88,bad"}, false},
		{Condition{Field: "score", Op: "gte", Value: "80"}, true},
	} {
		if got := (Evaluator{}).Match(profile, TargetingRule{All: []Condition{tc.condition}}); got != tc.want {
			t.Errorf("%+v got %v", tc, got)
		}
	}
}

func TestInvalidNoneConditionFailsClosed(t *testing.T) {
	profile := NewProfile("user", nil, map[string]string{"country": "156"})
	rule := TargetingRule{None: []Condition{{Field: "country", Op: "gte", Value: "1"}}}
	if (Evaluator{}).Match(profile, rule) {
		t.Fatal("invalid exclusion broadened audience")
	}
	failures := (Evaluator{}).Explain(profile, rule)
	if len(failures) != 1 || failures[0].Code != "invalid_condition" {
		t.Fatal(failures)
	}
}

func TestAgeRangeAndMemberDictionary(t *testing.T) {
	profile := NewProfile("user", nil, map[string]string{"age": "25", "member_level": "gold", "channel": "referral"})
	rule := TargetingRule{All: []Condition{{Field: "age", Op: "gte", Value: "18"}, {Field: "age", Op: "lte", Value: "35"}, {Field: "member_level", Op: "in", Value: "silver,gold"}, {Field: "channel", Op: "eq", Value: "referral"}}}
	if !(Evaluator{}).Match(profile, rule) {
		t.Fatal("valid demographic rule did not match")
	}
	profile.Fields["age"] = "25.5"
	if (Evaluator{}).Match(profile, rule) {
		t.Fatal("fractional age accepted")
	}
	rule.None = []Condition{{Field: "member_level", Op: "gte", Value: "silver"}}
	if (Evaluator{}).Match(profile, rule) {
		t.Fatal("invalid enum ordering accepted")
	}
}
