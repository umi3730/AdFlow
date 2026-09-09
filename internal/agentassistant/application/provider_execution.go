package application

import (
	"context"
	"fmt"
	"time"

	"github.com/umi3730/adflow/internal/agentassistant/domain"
)

type providerUsage struct {
	provider, model           string
	inputTokens, outputTokens int64
}

type providerCall func(context.Context) (providerUsage, error)

type fallbackReason uint8

const (
	noFallback fallbackReason = iota
	primaryFailed
	circuitOpen
)

// execute owns cancellation, circuit transitions, fallback and metrics for
// both operations. The typed callers keep their own result validation.
func (p *ResilientProvider) execute(ctx context.Context, prefix string, primary, fallback providerCall) (fallbackReason, error) {
	if err := ctx.Err(); err != nil {
		return noFallback, err
	}
	reason := circuitOpen
	if ticket, allowed := p.allowPrimary(p.now()); allowed {
		started := p.now()
		usage, err := primary(ctx)
		callerErr := ctx.Err()
		p.completePrimary(ticket, p.now(), err, callerErr)
		if callerErr != nil {
			p.observe(providerUsage{provider: p.config.PrimaryProvider, model: p.config.PrimaryModel}, prefix+"canceled", p.now().Sub(started))
			return noFallback, callerErr
		}
		if err == nil {
			p.observe(usage, prefix+"success", p.now().Sub(started))
			return noFallback, nil
		}
		p.observe(providerUsage{provider: p.config.PrimaryProvider, model: p.config.PrimaryModel}, prefix+"error", p.now().Sub(started))
		reason = primaryFailed
	}
	if err := ctx.Err(); err != nil {
		return noFallback, err
	}
	if !p.config.FallbackEnabled || fallback == nil {
		if reason == circuitOpen {
			return noFallback, domain.ErrProviderCircuitOpen
		}
		return noFallback, fmt.Errorf("%w: primary provider failed", domain.ErrProviderUnavailable)
	}
	started := p.now()
	usage, err := fallback(ctx)
	if callerErr := ctx.Err(); callerErr != nil {
		p.observe(providerUsage{provider: "fallback", model: "unknown"}, prefix+"fallback_canceled", p.now().Sub(started))
		return noFallback, callerErr
	}
	if err != nil {
		p.observe(providerUsage{provider: "fallback", model: "unknown"}, prefix+"fallback_error", p.now().Sub(started))
		return noFallback, fmt.Errorf("%w: primary and fallback providers failed", domain.ErrProviderUnavailable)
	}
	p.observe(usage, prefix+"fallback", p.now().Sub(started))
	return reason, nil
}

func (p *ResilientProvider) observe(usage providerUsage, outcome string, duration time.Duration) {
	if p.observer != nil {
		p.observer.ObserveAgentGeneration(usage.provider, usage.model, outcome, duration, usage.inputTokens, usage.outputTokens)
	}
}
