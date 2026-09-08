package application

import (
	"errors"
	"github.com/zhanghaiyang/adflow/internal/agentassistant/domain"
	"testing"
)

func TestAgentRejectsTextNumericComparison(t *testing.T) {
	draft := domain.Draft{Targeting: domain.TargetingRule{All: []domain.Condition{{Field: "country", Op: "gte", Value: "156"}}}, DailyBudgetFen: 100, ImpressionCostFen: 1, FrequencyLimit: 1, Explanation: "test", Provider: "test", Model: "test"}
	_, err := NewService(fixedProvider{draft: draft}).Generate(t.Context(), "生成国家定向规则")
	if !errors.Is(err, domain.ErrInvalidDraft) {
		t.Fatal("invalid model field comparison accepted", err)
	}
}
