package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/umi3730/adflow/internal/agentassistant/adapter/mock"
	"github.com/umi3730/adflow/internal/agentassistant/domain"
)

type switchableProvider struct {
	mu    sync.Mutex
	calls int
	fail  bool
}

func (p *switchableProvider) Generate(context.Context, string) (domain.Draft, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if p.fail {
		return domain.Draft{}, errors.New("provider down")
	}
	return domain.Draft{Provider: "primary", Model: "model", Targeting: domain.TargetingRule{All: []domain.Condition{{Tag: "anime"}}}, DailyBudgetFen: 1000, ImpressionCostFen: 10, FrequencyLimit: 3, Explanation: "ok"}, nil
}

func (p *switchableProvider) state() (int, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls, p.fail
}

func (p *switchableProvider) setFailure(fail bool) {
	p.mu.Lock()
	p.fail = fail
	p.mu.Unlock()
}

type recordingAgentObserver struct {
	mu          sync.Mutex
	outcomes    []string
	circuitOpen bool
}

func (o *recordingAgentObserver) ObserveAgentGeneration(_, _, outcome string, _ time.Duration, _, _ int64) {
	o.mu.Lock()
	o.outcomes = append(o.outcomes, outcome)
	o.mu.Unlock()
}

func (o *recordingAgentObserver) SetAgentCircuitOpen(open bool) {
	o.mu.Lock()
	o.circuitOpen = open
	o.mu.Unlock()
}

func TestResilientProviderFallsBackAndHalfOpenProbeRecovers(t *testing.T) {
	primary := &switchableProvider{fail: true}
	observer := &recordingAgentObserver{}
	provider, err := NewResilientProvider(primary, mock.NewProvider(), ResilienceConfig{
		PrimaryProvider: "primary", PrimaryModel: "model", FailureThreshold: 2,
		OpenDuration: time.Minute, FallbackEnabled: true,
	}, observer)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	provider.now = func() time.Time { return now }
	for index := 0; index < 3; index++ {
		draft, err := provider.Generate(t.Context(), "二次元策略用户")
		if err != nil || !draft.Fallback {
			t.Fatalf("fallback draft=%+v err=%v", draft, err)
		}
	}
	if calls, _ := primary.state(); calls != 2 {
		t.Fatalf("primary calls=%d, want 2", calls)
	}
	primary.setFailure(false)
	now = now.Add(time.Minute + time.Second)
	draft, err := provider.Generate(t.Context(), "二次元策略用户")
	if err != nil || draft.Fallback || draft.Provider != "primary" {
		t.Fatalf("recovery draft=%+v err=%v", draft, err)
	}
	observer.mu.Lock()
	defer observer.mu.Unlock()
	if observer.circuitOpen {
		t.Fatal("circuit should close after successful half-open probe")
	}
}

func TestResilientProviderWithoutFallbackReturnsExplicitErrors(t *testing.T) {
	primary := &switchableProvider{fail: true}
	provider, err := NewResilientProvider(primary, nil, ResilienceConfig{
		PrimaryProvider: "primary", PrimaryModel: "model", FailureThreshold: 1,
		OpenDuration: time.Minute, FallbackEnabled: false,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Generate(t.Context(), "生成规则草稿"); !errors.Is(err, domain.ErrProviderUnavailable) {
		t.Fatalf("first error=%v", err)
	}
	if _, err := provider.Generate(t.Context(), "生成规则草稿"); !errors.Is(err, domain.ErrProviderCircuitOpen) {
		t.Fatalf("open circuit error=%v", err)
	}
}
