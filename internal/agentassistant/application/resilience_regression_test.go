package application

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/umi3730/adflow/internal/agentassistant/domain"
)

type scriptedAgent struct {
	call func(context.Context) error
}

func (p scriptedAgent) Generate(ctx context.Context, _ string) (domain.Draft, error) {
	err := p.call(ctx)
	return domain.Draft{Provider: "test", Model: "test"}, err
}
func (p scriptedAgent) Diagnose(ctx context.Context, _ domain.DiagnosisContext) (domain.Diagnosis, error) {
	err := p.call(ctx)
	return domain.Diagnosis{Provider: "test", Model: "test", Summary: "summary",
		Recommendations: []domain.Recommendation{{Title: "title", Action: "action", EvidenceIDs: []string{"report"}}}}, err
}

var resilienceInput = domain.DiagnosisContext{Evidence: []domain.Evidence{{ID: "report"}}}

func newScriptedResilience(t *testing.T, primary, fallback domain.Provider, observer ProviderObserver) *ResilientProvider {
	t.Helper()
	p, err := NewResilientProvider(primary, fallback, ResilienceConfig{
		PrimaryProvider: "test", PrimaryModel: "test", FailureThreshold: 1,
		OpenDuration: time.Minute, FallbackEnabled: fallback != nil,
	}, observer)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

var agentOperations = []struct {
	name   string
	invoke func(*ResilientProvider, context.Context) error
}{
	{"generate", func(p *ResilientProvider, ctx context.Context) error {
		_, err := p.Generate(ctx, "generate draft")
		return err
	}},
	{"diagnose", func(p *ResilientProvider, ctx context.Context) error {
		_, err := p.Diagnose(ctx, resilienceInput)
		return err
	}},
}

func TestAgentCallerCancellationDoesNotTripCircuit(t *testing.T) {
	for _, op := range agentOperations {
		for _, returnsSuccess := range []bool{false, true} {
			t.Run(op.name+map[bool]string{true: "/late-success", false: "/error"}[returnsSuccess], func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				calls, fallbackCalls := 0, 0
				primary := scriptedAgent{call: func(context.Context) error {
					calls++
					if calls == 1 {
						cancel()
						if !returnsSuccess {
							return context.Canceled
						}
					}
					return nil
				}}
				fallback := scriptedAgent{call: func(context.Context) error { fallbackCalls++; return nil }}
				p := newScriptedResilience(t, primary, fallback, nil)
				if err := op.invoke(p, ctx); !errors.Is(err, context.Canceled) {
					t.Fatalf("canceled request: %v", err)
				}
				if err := op.invoke(p, t.Context()); err != nil {
					t.Fatal(err)
				}
				if calls != 2 || fallbackCalls != 0 {
					t.Fatalf("primary=%d fallback=%d", calls, fallbackCalls)
				}
			})
		}
	}
}

func TestAgentFallbackCancellationAndFailure(t *testing.T) {
	for _, op := range agentOperations {
		t.Run(op.name, func(t *testing.T) {
			for _, canceled := range []bool{false, true} {
				ctx, cancel := context.WithCancel(t.Context())
				observer := &recordingAgentObserver{}
				primary := scriptedAgent{call: func(context.Context) error { return errors.New("provider offline") }}
				fallback := scriptedAgent{call: func(context.Context) error {
					if canceled {
						cancel()
						return nil
					}
					return errors.New("private implementation detail")
				}}
				p := newScriptedResilience(t, primary, fallback, observer)
				err := op.invoke(p, ctx)
				cancel()
				if canceled {
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("late fallback result: %v", err)
					}
				} else {
					if !errors.Is(err, domain.ErrProviderUnavailable) || strings.Contains(err.Error(), "private") {
						t.Fatalf("fallback error was not classified/sanitized: %v", err)
					}
					prefix := ""
					if op.name == "diagnose" {
						prefix = "diagnosis_"
					}
					if len(observer.outcomes) != 2 || observer.outcomes[1] != prefix+"fallback_error" {
						t.Fatalf("missing fallback failure metric: %v", observer.outcomes)
					}
				}
			}
		})
	}
}

func TestAgentCanceledHalfOpenProbeAllowsAnotherProbe(t *testing.T) {
	for _, op := range agentOperations {
		t.Run(op.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			calls := 0
			primary := scriptedAgent{call: func(context.Context) error {
				calls++
				switch calls {
				case 1:
					return errors.New("offline")
				case 2:
					cancel()
					return context.Canceled
				}
				return nil
			}}
			p := newScriptedResilience(t, primary, nil, nil)
			now := time.Now()
			p.now = func() time.Time { return now }
			if err := op.invoke(p, t.Context()); !errors.Is(err, domain.ErrProviderUnavailable) {
				t.Fatal(err)
			}
			now = now.Add(2 * time.Minute)
			if err := op.invoke(p, ctx); !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if err := op.invoke(p, t.Context()); err != nil {
				t.Fatalf("canceled probe blocked recovery: %v", err)
			}
			if calls != 3 {
				t.Fatalf("calls=%d", calls)
			}
		})
	}
}

func TestAgentLateSuccessCannotCloseNewerCircuit(t *testing.T) {
	for _, op := range agentOperations {
		t.Run(op.name, func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			var calls atomic.Int32
			primary := scriptedAgent{call: func(context.Context) error {
				if calls.Add(1) == 1 {
					close(started)
					<-release
					return nil
				}
				return errors.New("offline")
			}}
			p := newScriptedResilience(t, primary, nil, nil)
			done := make(chan error, 1)
			go func() { done <- op.invoke(p, t.Context()) }()
			<-started
			failure := op.invoke(p, t.Context())
			close(release)
			late := <-done
			if !errors.Is(failure, domain.ErrProviderUnavailable) || late != nil {
				t.Fatalf("failure=%v late=%v", failure, late)
			}
			if err := op.invoke(p, t.Context()); !errors.Is(err, domain.ErrProviderCircuitOpen) {
				t.Fatalf("late success closed circuit: %v", err)
			}
			if calls.Load() != 2 {
				t.Fatalf("primary called through open circuit: %d", calls.Load())
			}
		})
	}
}

func TestAgentLateFailureCannotReopenRecoveredCircuit(t *testing.T) {
	for _, op := range agentOperations {
		t.Run(op.name, func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			var calls atomic.Int32
			primary := scriptedAgent{call: func(context.Context) error {
				switch calls.Add(1) {
				case 1:
					close(started)
					<-release
					return errors.New("late failure")
				case 2:
					return errors.New("offline")
				}
				return nil
			}}
			p := newScriptedResilience(t, primary, nil, nil)
			var nanos atomic.Int64
			nanos.Store(time.Now().UnixNano())
			p.now = func() time.Time { return time.Unix(0, nanos.Load()) }
			done := make(chan error, 1)
			go func() { done <- op.invoke(p, t.Context()) }()
			<-started
			failure := op.invoke(p, t.Context())
			nanos.Add(int64(2 * time.Minute))
			recovered := op.invoke(p, t.Context())
			close(release)
			late := <-done
			if !errors.Is(failure, domain.ErrProviderUnavailable) || recovered != nil || !errors.Is(late, domain.ErrProviderUnavailable) {
				t.Fatalf("failure=%v recovered=%v late=%v", failure, recovered, late)
			}
			if err := op.invoke(p, t.Context()); err != nil {
				t.Fatalf("late failure reopened circuit: %v", err)
			}
		})
	}
}

func TestAgentHalfOpenAllowsOnlyOneProbe(t *testing.T) {
	for _, op := range agentOperations {
		t.Run(op.name, func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			var calls atomic.Int32
			primary := scriptedAgent{call: func(context.Context) error {
				switch calls.Add(1) {
				case 1:
					return errors.New("offline")
				case 2:
					close(started)
					<-release
				}
				return nil
			}}
			p := newScriptedResilience(t, primary, nil, nil)
			now := time.Now()
			p.now = func() time.Time { return now }
			if err := op.invoke(p, t.Context()); !errors.Is(err, domain.ErrProviderUnavailable) {
				t.Fatal(err)
			}
			now = now.Add(2 * time.Minute)
			done := make(chan error, 1)
			go func() { done <- op.invoke(p, t.Context()) }()
			<-started
			concurrent := op.invoke(p, t.Context())
			close(release)
			probe := <-done
			if !errors.Is(concurrent, domain.ErrProviderCircuitOpen) || probe != nil || calls.Load() != 2 {
				t.Fatalf("concurrent=%v probe=%v calls=%d", concurrent, probe, calls.Load())
			}
		})
	}
}

type invalidDiagnosticFallback struct{ scriptedAgent }

func (invalidDiagnosticFallback) Diagnose(context.Context, domain.DiagnosisContext) (domain.Diagnosis, error) {
	return domain.Diagnosis{Summary: "unsupported recommendation"}, nil
}

func TestAgentDiagnosticFallbackMustPassEvidenceValidation(t *testing.T) {
	primary := scriptedAgent{call: func(context.Context) error { return errors.New("offline") }}
	observer := &recordingAgentObserver{}
	p := newScriptedResilience(t, primary, invalidDiagnosticFallback{}, observer)
	if _, err := p.Diagnose(t.Context(), resilienceInput); !errors.Is(err, domain.ErrProviderUnavailable) {
		t.Fatalf("invalid fallback was accepted: %v", err)
	}
	if len(observer.outcomes) != 2 || observer.outcomes[1] != "diagnosis_fallback_error" {
		t.Fatalf("invalid fallback counted as success: %v", observer.outcomes)
	}
}
