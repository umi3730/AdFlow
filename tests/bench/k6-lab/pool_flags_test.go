package main

import "testing"

func TestPoolFlagsKeepIdleWithinOpenLimit(t *testing.T) {
	for _, idle := range []int{10, 20} {
		if err := validatePoolFlags(30, idle, true); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		open, idle int
		enabled    bool
	}{{30, 31, true}, {30, -1, true}, {30, 20, false}, {0, 20, true}, {101, 10, true}} {
		if err := validatePoolFlags(c.open, c.idle, c.enabled); err == nil {
			t.Fatalf("invalid pool flags accepted: %+v", c)
		}
	}
	if err := validatePoolFlags(0, 10, false); err != nil {
		t.Fatal("ordinary runs must stay compatible", err)
	}
}
