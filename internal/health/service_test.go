package health

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeChecker struct{ err error }

func (f fakeChecker) PingContext(context.Context) error { return f.err }

func TestCheckReportsDependencyFailure(t *testing.T) {
	service := NewService(50*time.Millisecond, map[string]Checker{
		"mysql": fakeChecker{},
		"redis": fakeChecker{err: errors.New("unavailable")},
	})
	status := service.Check(context.Background())
	if status.Ready {
		t.Fatal("expected service to be not ready")
	}
	if status.Dependencies["mysql"] != "up" || status.Dependencies["redis"] != "down" {
		t.Fatalf("unexpected dependencies: %+v", status.Dependencies)
	}
}
