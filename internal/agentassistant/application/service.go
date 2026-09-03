package application

import (
	"context"
	"strings"
	"time"

	agentdomain "github.com/zhanghaiyang/adflow/internal/agentassistant/domain"
	campaigndomain "github.com/zhanghaiyang/adflow/internal/campaign/domain"
)

type Service struct {
	provider agentdomain.Provider
	now      func() time.Time
}

func NewService(provider agentdomain.Provider) *Service {
	return &Service{provider: provider, now: time.Now}
}

func (s *Service) Generate(ctx context.Context, prompt string) (agentdomain.Draft, error) {
	prompt = strings.TrimSpace(prompt)
	if length := len([]rune(prompt)); length < 5 || length > 2000 {
		return agentdomain.Draft{}, agentdomain.ErrInvalidPrompt
	}
	draft, err := s.provider.Generate(ctx, prompt)
	if err != nil {
		return agentdomain.Draft{}, err
	}
	all := mapConditions(draft.Targeting.All)
	anyOf := mapConditions(draft.Targeting.Any)
	none := mapConditions(draft.Targeting.None)
	if _, err := campaigndomain.NewTargetingRule(all, anyOf, none); err != nil {
		return agentdomain.Draft{}, agentdomain.ErrInvalidDraft
	}
	if _, err := campaigndomain.NewVersion(1, mustRule(all, anyOf, none), draft.DailyBudgetFen, draft.ImpressionCostFen, draft.FrequencyLimit, s.now()); err != nil {
		return agentdomain.Draft{}, agentdomain.ErrInvalidDraft
	}
	draft.GeneratedAt = s.now().UTC()
	if draft.PromptVersion == "" {
		draft.PromptVersion = "rule-draft-v1"
	}
	return draft, nil
}

func mapConditions(input []agentdomain.Condition) []campaigndomain.Condition {
	result := make([]campaigndomain.Condition, 0, len(input))
	for _, condition := range input {
		result = append(result, campaigndomain.Condition{Tag: condition.Tag, Field: condition.Field, Op: condition.Op, Value: condition.Value})
	}
	return result
}

func mustRule(all, anyOf, none []campaigndomain.Condition) campaigndomain.TargetingRule {
	rule, _ := campaigndomain.NewTargetingRule(all, anyOf, none)
	return rule
}
