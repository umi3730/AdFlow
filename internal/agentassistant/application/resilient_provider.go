package application

import (
	"context"
	"errors"
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

	mu         sync.Mutex
	failures   int
	openUntil  time.Time
	probing    bool
	generation uint64
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
	var draft domain.Draft
	call := func(provider domain.Provider) providerCall {
		if provider == nil {
			return nil
		}
		return func(ctx context.Context) (providerUsage, error) {
			var err error
			draft, err = provider.Generate(ctx, prompt)
			return providerUsage{draft.Provider, draft.Model, draft.InputTokens, draft.OutputTokens}, err
		}
	}
	reason, err := p.execute(ctx, "", call(p.primary), call(p.fallback))
	if err != nil {
		return domain.Draft{}, err
	}
	if reason != noFallback {
		draft.Fallback = true
		warning := "主模型暂时不可用，已降级到本地规则解析器，请人工复核。"
		if reason == circuitOpen {
			warning = "主模型熔断中，已使用本地规则解析器，请人工复核。"
		}
		draft.Warnings = append(draft.Warnings, warning)
	}
	return draft, nil
}

// A ticket ties completion to the circuit generation that admitted the call.
// Calls already in flight cannot close a newly opened circuit or reopen one
// that has recovered through a later probe.
type circuitTicket struct {
	generation uint64
	probe      bool
}

func (p *ResilientProvider) allowPrimary(now time.Time) (circuitTicket, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	ticket := circuitTicket{generation: p.generation}
	if p.openUntil.IsZero() {
		return ticket, true
	}
	if now.Before(p.openUntil) || p.probing {
		return ticket, false
	}
	p.probing = true
	ticket.probe = true
	return ticket, true
}

func (p *ResilientProvider) completePrimary(ticket circuitTicket, now time.Time, err, callerErr error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if ticket.generation != p.generation {
		return
	}
	// Caller cancellation says nothing about the provider's health. Release a
	// half-open probe so the next live caller can attempt recovery immediately.
	if callerErr != nil {
		if ticket.probe {
			p.probing = false
		}
		return
	}
	if err == nil {
		p.failures = 0
		p.openUntil = time.Time{}
		p.probing = false
		if ticket.probe {
			p.generation++
		}
	} else {
		p.failures++
		if ticket.probe || p.failures >= p.config.FailureThreshold {
			p.openUntil = now.Add(p.config.OpenDuration)
			p.probing = false
			p.generation++
		}
	}
	// Serialize gauge updates with state transitions, including concurrent calls.
	if p.observer != nil {
		p.observer.SetAgentCircuitOpen(!p.openUntil.IsZero())
	}
}
