package application

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zhanghaiyang/adflow/internal/decision/domain"
)

type immediateEngine struct{}

func (immediateEngine) Decide(_ context.Context, request domain.Request) (domain.Result, error) {
	return domain.Result{RequestID: request.RequestID}, nil
}

type blockingEngine struct {
	entered chan struct{}
	release chan struct{}
}

func (e blockingEngine) Decide(ctx context.Context, request domain.Request) (domain.Result, error) {
	select {
	case e.entered <- struct{}{}:
	default:
	}
	select {
	case <-e.release:
		return domain.Result{RequestID: request.RequestID}, nil
	case <-ctx.Done():
		return domain.Result{}, ctx.Err()
	}
}

func admissionConfig() AdmissionConfig {
	return AdmissionConfig{RatePerSecond: 100000, Burst: 1000, MaxInFlight: 4, QueueTimeout: 100 * time.Millisecond, RequestTimeout: time.Second}
}

func TestAdmissionRateLimitsBurst(t *testing.T) {
	cfg := admissionConfig()
	cfg.RatePerSecond = 1
	cfg.Burst = 1
	service, err := NewAdmissionService(immediateEngine{}, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Decide(t.Context(), domain.Request{RequestID: "first"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Decide(t.Context(), domain.Request{RequestID: "second"}); !errors.Is(err, domain.ErrRateLimited) {
		t.Fatalf("second request error=%v", err)
	}
}

func TestAdmissionRejectsWhenConcurrencyQueueExpires(t *testing.T) {
	engine := blockingEngine{entered: make(chan struct{}, 1), release: make(chan struct{})}
	cfg := admissionConfig()
	cfg.MaxInFlight = 1
	cfg.QueueTimeout = 5 * time.Millisecond
	service, err := NewAdmissionService(engine, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, callErr := service.Decide(context.Background(), domain.Request{RequestID: "first"})
		done <- callErr
	}()
	<-engine.entered
	if _, err := service.Decide(t.Context(), domain.Request{RequestID: "second"}); !errors.Is(err, domain.ErrOverloaded) {
		t.Fatalf("second request error=%v", err)
	}
	close(engine.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestAdmissionAppliesRequestDeadline(t *testing.T) {
	engine := blockingEngine{entered: make(chan struct{}, 1), release: make(chan struct{})}
	cfg := admissionConfig()
	cfg.RequestTimeout = 5 * time.Millisecond
	service, err := NewAdmissionService(engine, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Decide(t.Context(), domain.Request{RequestID: "slow"}); !errors.Is(err, domain.ErrDecisionTimeout) {
		t.Fatalf("slow request error=%v", err)
	}
}

type countingEngine struct {
	current atomic.Int64
	maximum atomic.Int64
}

func (e *countingEngine) Decide(_ context.Context, request domain.Request) (domain.Result, error) {
	current := e.current.Add(1)
	defer e.current.Add(-1)
	for {
		maximum := e.maximum.Load()
		if current <= maximum || e.maximum.CompareAndSwap(maximum, current) {
			break
		}
	}
	time.Sleep(2 * time.Millisecond)
	return domain.Result{RequestID: request.RequestID}, nil
}

func TestAdmissionCapsConcurrentExecution(t *testing.T) {
	engine := &countingEngine{}
	cfg := admissionConfig()
	cfg.MaxInFlight = 4
	cfg.QueueTimeout = time.Second
	service, err := NewAdmissionService(engine, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	errorsFound := make(chan error, 40)
	for index := 0; index < 40; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, callErr := service.Decide(context.Background(), domain.Request{RequestID: "concurrent"})
			if callErr != nil {
				errorsFound <- callErr
			}
		}()
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Fatal(err)
	}
	if maximum := engine.maximum.Load(); maximum > 4 || maximum < 2 {
		t.Fatalf("maximum concurrency=%d", maximum)
	}
}

func BenchmarkAdmissionFastPath(b *testing.B) {
	cfg := admissionConfig()
	cfg.RatePerSecond = 1e12
	cfg.Burst = b.N + 1
	cfg.MaxInFlight = 256
	service, err := NewAdmissionService(immediateEngine{}, cfg, nil)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.RunParallel(func(parallel *testing.PB) {
		for parallel.Next() {
			if _, err := service.Decide(context.Background(), domain.Request{RequestID: "benchmark"}); err != nil {
				b.Fatal(err)
			}
		}
	})
}
