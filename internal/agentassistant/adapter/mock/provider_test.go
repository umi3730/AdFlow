package mock

import "testing"

func TestGeneralAudienceKeywordsAndExclusion(t *testing.T) {
	draft, err := NewProvider().Generate(t.Context(), "对数码或购物感兴趣的活跃安卓用户，排除付费用户")
	if err != nil {
		t.Fatal(err)
	}
	if len(draft.Targeting.Any) != 2 || draft.Targeting.Any[0].Tag != "tech_interest" || draft.Targeting.Any[1].Tag != "shopping_interest" {
		t.Fatal(draft.Targeting)
	}
	if len(draft.Targeting.None) != 1 || draft.Targeting.None[0].Tag != "paying_user" {
		t.Fatal(draft.Targeting)
	}
	for _, condition := range draft.Targeting.All {
		if condition.Tag == "paying_user" {
			t.Fatal("excluded tag also required")
		}
	}
}

func TestGamingInterestIsAvailableAlongsideTechnology(t *testing.T) {
	draft, err := NewProvider().Generate(t.Context(), "对数码或游戏感兴趣的活跃安卓用户")
	if err != nil {
		t.Fatal(err)
	}
	if len(draft.Targeting.Any) != 2 || draft.Targeting.Any[1].Tag != "gaming_interest" {
		t.Fatal(draft.Targeting)
	}
}
