package application

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/zhanghaiyang/adflow/internal/decision/domain"
)

type DecisionEngine interface {
	Decide(context.Context, domain.Request) (domain.Result, error)
}

type AdmissionConfig struct {
	MaxInFlight    int
	QueueTimeout   time.Duration
	RequestTimeout time.Duration
}

type RateLimiter interface {
	Allow(context.Context, string) (bool, error)
}

type AdmissionObserver interface {
	ObserveDecisionAdmission(result string, queueDuration time.Duration)
	ObserveDecisionTimeout()
	AddDecisionInFlight(delta float64)
}

type AdmissionService struct {
	next           DecisionEngine
	limiter        RateLimiter
	slots          chan struct{}
	queueTimeout   time.Duration
	requestTimeout time.Duration
	observer       AdmissionObserver
}

func NewAdmissionService(next DecisionEngine, limiter RateLimiter, cfg AdmissionConfig, observer AdmissionObserver) (*AdmissionService, error) {
	if next == nil || limiter == nil || cfg.MaxInFlight <= 0 || cfg.QueueTimeout <= 0 || cfg.RequestTimeout <= 0 {
		return nil, errors.New("admission control requires positive limits and a decision engine")
	}
	return &AdmissionService{
		next: next, limiter: limiter, slots: make(chan struct{}, cfg.MaxInFlight),
		queueTimeout: cfg.QueueTimeout, requestTimeout: cfg.RequestTimeout, observer: observer,
	}, nil
}

func (s *AdmissionService) Decide(ctx context.Context, request domain.Request) (domain.Result, error) {
	started := time.Now()
	if err := ctx.Err(); err != nil {
		s.observe("canceled", 0)
		return domain.Result{}, err
	}
	allowed, err := s.limiter.Allow(ctx, request.RequestID)
	if err != nil {
		if ctx.Err() != nil {
			s.observe("canceled", time.Since(started))
			return domain.Result{}, ctx.Err()
		}
		s.observe("limiter_error", 0)
		return domain.Result{}, fmt.Errorf("%w: %v", domain.ErrAdmissionUnavailable, err)
	}
	if !allowed {
		s.observe("rate_limited", 0)
		return domain.Result{}, domain.ErrRateLimited
	}
	queueDuration, err := s.acquire(ctx, started)
	if errors.Is(err, domain.ErrOverloaded) {
		s.observe("overloaded", time.Since(started))
		return domain.Result{}, domain.ErrOverloaded
	}
	if err != nil {
		s.observe("canceled", time.Since(started))
		return domain.Result{}, err
	}
	s.observe("accepted", queueDuration)
	if s.observer != nil {
		s.observer.AddDecisionInFlight(1)
	}
	defer func() {
		<-s.slots
		if s.observer != nil {
			s.observer.AddDecisionInFlight(-1)
		}
	}()

	requestContext, cancel := context.WithTimeout(ctx, s.requestTimeout)
	defer cancel()
	result, err := s.next.Decide(requestContext, request)
	if errors.Is(requestContext.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		if s.observer != nil {
			s.observer.ObserveDecisionTimeout()
		}
		cause := err
		if cause == nil {
			cause = requestContext.Err()
		}
		return domain.Result{}, fmt.Errorf("%w: %v", domain.ErrDecisionTimeout, cause)
	}
	if requestContext.Err() != nil {
		return domain.Result{}, requestContext.Err()
	}
	return result, err
}

func (s *AdmissionService) acquire(ctx context.Context, started time.Time) (time.Duration, error) {
	select {
	case s.slots <- struct{}{}:
		return time.Since(started), nil
	default:
	}
	timer := time.NewTimer(s.queueTimeout)
	defer timer.Stop()
	select {
	case s.slots <- struct{}{}:
		return time.Since(started), nil
	case <-timer.C:
		return 0, domain.ErrOverloaded
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func (s *AdmissionService) observe(result string, queueDuration time.Duration) {
	if s.observer != nil {
		s.observer.ObserveDecisionAdmission(result, queueDuration)
	}
}

type TokenBucketLimiter struct {
	mu       sync.Mutex
	rate     float64
	capacity float64
	tokens   float64
	last     time.Time
}

func NewTokenBucketLimiter(rate float64, burst int) (*TokenBucketLimiter, error) {
	if rate <= 0 || burst <= 0 {
		return nil, errors.New("token bucket requires a positive rate and burst")
	}
	return &TokenBucketLimiter{rate: rate, capacity: float64(burst), tokens: float64(burst)}, nil
}

func (b *TokenBucketLimiter) Allow(_ context.Context, _ string) (bool, error) {
	now := time.Now()
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.last.IsZero() {
		b.last = now
	} else if now.After(b.last) {
		b.tokens = math.Min(b.capacity, b.tokens+now.Sub(b.last).Seconds()*b.rate)
		b.last = now
	}
	if b.tokens < 1 {
		return false, nil
	}
	b.tokens--
	return true, nil
}
