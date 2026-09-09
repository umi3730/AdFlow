package mock

import (
	"context"
	"fmt"
	"github.com/umi3730/adflow/internal/agentassistant/domain"
)

func (p *Provider) Diagnose(ctx context.Context, input domain.DiagnosisContext) (domain.Diagnosis, error) {
	if err := ctx.Err(); err != nil {
		return domain.Diagnosis{}, err
	}
	result := domain.Diagnosis{Provider: "mock", Model: "local-delivery-diagnostics", PromptVersion: "delivery-diagnosis-v1", Summary: fmt.Sprintf("已检查投放数据与配置，整理出 %d 项依据。建议按以下顺序排查和调整。", len(input.Evidence)), Recommendations: []domain.Recommendation{}}
	// Specific configuration/request findings take precedence over general metrics.
	for i := len(input.Evidence) - 1; i >= 0 && len(result.Recommendations) < 6; i-- {
		e := input.Evidence[i]
		if e.Suggestion != "" {
			result.Recommendations = append(result.Recommendations, domain.Recommendation{Title: e.Title, Action: e.Suggestion, EvidenceIDs: []string{e.ID}})
		}
	}
	return result, domain.ValidateDiagnosis(result, input)
}
