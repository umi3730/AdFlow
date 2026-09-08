package domain

import "testing"

func TestExplainReportsMissingFieldAndExclusions(t *testing.T) {
	profile := NewProfile("user", []string{"anime", "installed_target_game"}, map[string]string{"device": "android", "score": "20"})
	rule := TargetingRule{
		All:  []Condition{{Tag: "strategy_game"}, {Field: "platform", Op: "eq", Value: "android"}, {Field: "score", Op: "gte", Value: "80"}},
		Any:  []Condition{{Tag: "anime"}, {Tag: "active_7d"}},
		None: []Condition{{Tag: "installed_target_game"}},
	}
	failures := (Evaluator{}).Explain(profile, rule)
	if len(failures) != 4 || failures[0].Code != "missing_tag" || failures[1].Code != "missing_field" || failures[2].Actual != "20" || failures[3].Code != "excluded_condition" {
		t.Fatalf("failures=%+v", failures)
	}
	if (Evaluator{}).Match(profile, rule) {
		t.Fatal("mismatching rule accepted")
	}
}

func TestExplainAgreesWithMatch(t *testing.T) {
	profile := NewProfile("user", []string{"anime"}, map[string]string{"device": "android"})
	for _, rule := range []TargetingRule{
		{All: []Condition{{Tag: "anime"}}},
		{Any: []Condition{{Tag: "unknown"}}},
		{Any: []Condition{{Tag: "unknown"}, {Tag: "anime"}}},
		{None: []Condition{{Tag: "anime"}}},
		{All: []Condition{{Field: "device", Op: "eq", Value: "ios"}}},
	} {
		if ((Evaluator{}).Match(profile, rule)) != (len((Evaluator{}).Explain(profile, rule)) == 0) {
			t.Fatalf("disagreement for %+v", rule)
		}
	}
}
