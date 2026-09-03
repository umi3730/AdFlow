package domain

import "testing"

func TestEvaluatorCombinesAllAnyAndNone(t *testing.T) {
	profile := NewProfile("user-1", []string{"anime", "strategy"}, map[string]string{"device": "android", "score": "88"})
	rule := TargetingRule{
		All:  []Condition{{Tag: "anime"}, {Field: "device", Op: "eq", Value: "android"}},
		Any:  []Condition{{Tag: "rpg"}, {Field: "score", Op: "gte", Value: "80"}},
		None: []Condition{{Tag: "installed_target_game"}},
	}
	if !((Evaluator{}).Match(profile, rule)) {
		t.Fatal("expected profile to match")
	}
	profile.Tags["installed_target_game"] = struct{}{}
	if (Evaluator{}).Match(profile, rule) {
		t.Fatal("expected none condition to reject profile")
	}
}

func BenchmarkEvaluatorMatch(b *testing.B) {
	profile := NewProfile("user-1", []string{"anime", "strategy", "active_7d"}, map[string]string{"device": "android", "score": "88"})
	rule := TargetingRule{
		All:  []Condition{{Tag: "anime"}, {Tag: "strategy"}, {Field: "device", Op: "eq", Value: "android"}},
		Any:  []Condition{{Tag: "rpg"}, {Field: "score", Op: "gte", Value: "80"}},
		None: []Condition{{Tag: "installed_target_game"}},
	}
	evaluator := Evaluator{}
	b.ResetTimer()
	for b.Loop() {
		if !evaluator.Match(profile, rule) {
			b.Fatal("expected match")
		}
	}
}
