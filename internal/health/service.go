package health

import (
	"context"
	"sync"
	"time"
)

type Checker interface {
	PingContext(context.Context) error
}

type Status struct {
	Ready        bool              `json:"ready"`
	Dependencies map[string]string `json:"dependencies"`
}

type Service struct {
	timeout  time.Duration
	checkers map[string]Checker
}

func NewService(timeout time.Duration, checkers map[string]Checker) *Service {
	return &Service{timeout: timeout, checkers: checkers}
}

func (s *Service) Check(ctx context.Context) Status {
	status := Status{Ready: true, Dependencies: make(map[string]string, len(s.checkers))}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for name, checker := range s.checkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			checkCtx, cancel := context.WithTimeout(ctx, s.timeout)
			defer cancel()
			result := "up"
			if err := checker.PingContext(checkCtx); err != nil {
				result = "down"
			}
			mu.Lock()
			status.Dependencies[name] = result
			if result != "up" {
				status.Ready = false
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	return status
}
