package main

import (
	"testing"
	"time"
)

func TestClockGuardDetectsWallTimeSteps(t *testing.T) {
	if got := clockDrift(300*time.Millisecond, 1100*time.Millisecond); got != 800*time.Millisecond {
		t.Fatal(got)
	}
	if got := clockDrift(1900*time.Millisecond, 1100*time.Millisecond); got != 800*time.Millisecond {
		t.Fatal(got)
	}
	if got := clockDrift(time.Second, time.Second); got != 0 {
		t.Fatal(got)
	}
	g := newClockGuard()
	observation := g.Stop()
	if !observation.Stable || observation.Samples < 1 {
		t.Fatalf("%+v", observation)
	}
	if second := g.Stop(); second != observation {
		t.Fatal("stop must be idempotent")
	}
}
