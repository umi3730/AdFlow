package application

import (
	"context"

	"github.com/umi3730/adflow/internal/agentassistant/domain"
)

func (p *ResilientProvider) Diagnose(ctx context.Context, input domain.DiagnosisContext) (domain.Diagnosis, error) {
	if err := ctx.Err(); err != nil {
		return domain.Diagnosis{}, err
	}
	primary, ok := p.primary.(domain.DiagnosticProvider)
	if !ok {
		return domain.Diagnosis{}, domain.ErrProviderUnavailable
	}
	fallback, _ := p.fallback.(domain.DiagnosticProvider)
	var diagnosis domain.Diagnosis
	call := func(provider domain.DiagnosticProvider) providerCall {
		if provider == nil {
			return nil
		}
		return func(ctx context.Context) (providerUsage, error) {
			var err error
			diagnosis, err = provider.Diagnose(ctx, input)
			if err == nil {
				err = domain.ValidateDiagnosis(diagnosis, input)
			}
			return providerUsage{diagnosis.Provider, diagnosis.Model, diagnosis.InputTokens, diagnosis.OutputTokens}, err
		}
	}
	reason, err := p.execute(ctx, "diagnosis_", call(primary), call(fallback))
	if err != nil {
		return domain.Diagnosis{}, err
	}
	if reason != noFallback {
		diagnosis.Fallback = true
	}
	return diagnosis, nil
}
