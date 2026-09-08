package application

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/umi3730/adflow/internal/agentassistant/domain"
)

type ProviderObserver interface {
	ObserveAgentGeneration(provider, model, outcome string, duration time.Duration, inputTokens, outputTokens int64)
	SetAgentCircuitOpen(open bool)
}

type ResilienceConfig struct {
	PrimaryProvider  string
	PrimaryModel     string
	FailureThreshold int
	OpenDuration     time.Duration
	FallbackEnabled  bool
}

type ResilientProvider struct {
	primary  domain.Provider
	fallback domain.Provider
	config   ResilienceConfig
	observer ProviderObserver
	now      func() time.Time

	mu        sync.Mutex
	failures  int
	openUntil time.Time
	probing   bool
}

func NewResilientProvider(primary, fallback domain.Provider, config ResilienceConfig, observer ProviderObserver) (*ResilientProvider, error) {
	if primary == nil || config.PrimaryProvider == "" || config.PrimaryModel == "" || config.FailureThreshold <= 0 || config.OpenDuration <= 0 {
		return nil, errors.New("resilient provider configuration is invalid")
	}
	if config.FallbackEnabled && fallback == nil {
		return nil, errors.New("enabled provider fallback is missing")
	}
	return &ResilientProvider{primary: primary, fallback: fallback, config: config, observer: observer, now: time.Now}, nil
}

func (p *ResilientProvider) Generate(ctx context.Context, prompt string) (domain.Draft, error) {
	if err := ctx.Err(); err != nil {
		return domain.Draft{}, err
	}
	if !p.allowPrimary(p.now()) {
		return p.useFallback(ctx, prompt, "主模型熔断中，已使用本地规则解析器，请人工复核。")
	}
	started := p.now()
	draft, err := p.primary.Generate(ctx, prompt)
	duration := p.now().Sub(started)
	if err == nil {
		p.recordSuccess()
		p.observe(draft.Provider, draft.Model, "success", duration, draft.InputTokens, draft.OutputTokens)
		return draft, nil
	}
	p.observe(p.config.PrimaryProvider, p.config.PrimaryModel, "error", duration, 0, 0)
	p.recordFailure(p.now())
	if ctx.Err() != nil {
		return domain.Draft{}, ctx.Err()
	}
	if !p.config.FallbackEnabled {
		return domain.Draft{}, fmt.Errorf("%w: primary provider failed", domain.ErrProviderUnavailable)
	}
	return p.useFallback(ctx, prompt, "主模型暂时不可用，已降级到本地规则解析器，请人工复核。")
}

func (p *ResilientProvider) allowPrimary(now time.Time) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.openUntil.IsZero() {
		return true
	}
	if now.Before(p.openUntil) || p.probing {
		return false
	}
	p.probing = true
	return true
}

func (p *ResilientProvider) recordSuccess() {
	p.mu.Lock()
	p.failures = 0
	p.openUntil = time.Time{}
	p.probing = false
	p.mu.Unlock()
	if p.observer != nil {
		p.observer.SetAgentCircuitOpen(false)
	}
}

func (p *ResilientProvider) recordFailure(now time.Time) {
	p.mu.Lock()
	p.failures++
	if p.probing || p.failures >= p.config.FailureThreshold {
		p.openUntil = now.Add(p.config.OpenDuration)
		p.probing = false
	}
	open := !p.openUntil.IsZero()
	p.mu.Unlock()
	if open && p.observer != nil {
		p.observer.SetAgentCircuitOpen(true)
	}
}

func (p *ResilientProvider) useFallback(ctx context.Context, prompt, warning string) (domain.Draft, error) {
	if err := ctx.Err(); err != nil {
		return domain.Draft{}, err
	}
	if !p.config.FallbackEnabled || p.fallback == nil {
		return domain.Draft{}, domain.ErrProviderCircuitOpen
	}
	started := p.now()
	draft, err := p.fallback.Generate(ctx, prompt)
	duration := p.now().Sub(started)
	if err != nil {
		p.observe("fallback", "unknown", "error", duration, 0, 0)
		return domain.Draft{}, fmt.Errorf("%w: primary and fallback providers failed", domain.ErrProviderUnavailable)
	}
	if err := ctx.Err(); err != nil {
		return domain.Draft{}, err
	}
	draft.Fallback = true
	draft.Warnings = append(draft.Warnings, warning)
	p.observe(draft.Provider, draft.Model, "fallback", duration, draft.InputTokens, draft.OutputTokens)
	return draft, nil
}

func (p *ResilientProvider) observe(provider, model, outcome string, duration time.Duration, inputTokens, outputTokens int64) {
	if p.observer != nil {
		p.observer.ObserveAgentGeneration(provider, model, outcome, duration, inputTokens, outputTokens)
	}
}
