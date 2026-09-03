package mock

import (
	"context"
	"strings"

	"github.com/zhanghaiyang/adflow/internal/agentassistant/domain"
)

type Provider struct{}

func NewProvider() *Provider { return &Provider{} }

func (*Provider) Generate(_ context.Context, prompt string) (domain.Draft, error) {
	lower := strings.ToLower(prompt)
	all := make([]domain.Condition, 0, 4)
	none := make([]domain.Condition, 0, 1)
	if strings.Contains(lower, "二次元") || strings.Contains(lower, "anime") {
		all = append(all, domain.Condition{Tag: "anime"})
	}
	if strings.Contains(lower, "策略") || strings.Contains(lower, "strategy") {
		all = append(all, domain.Condition{Tag: "strategy_game"})
	}
	if strings.Contains(lower, "活跃") || strings.Contains(lower, "active") {
		all = append(all, domain.Condition{Tag: "active_7d"})
	}
	if strings.Contains(lower, "安卓") || strings.Contains(lower, "android") {
		all = append(all, domain.Condition{Field: "device", Op: "eq", Value: "android"})
	}
	if strings.Contains(lower, "未安装") || strings.Contains(lower, "排除已安装") || strings.Contains(lower, "排除已经安装") || strings.Contains(lower, "not installed") {
		none = append(none, domain.Condition{Tag: "installed_target_game"})
	}
	if len(all)+len(none) == 0 {
		all = append(all, domain.Condition{Tag: "general_audience"})
	}
	return domain.Draft{
		Targeting:      domain.TargetingRule{All: all, None: none},
		DailyBudgetFen: 100000, ImpressionCostFen: 100, FrequencyLimit: 3,
		Explanation: "根据自然语言中的兴趣、活跃度和设备关键词生成结构化定向草稿。",
		Warnings:    []string{"当前为本地Mock Provider，发布前必须人工确认预算、频控和目标人群。"},
		Provider:    "local-mock", Model: "deterministic-rule-parser", PromptVersion: "rule-draft-v1",
	}, nil
}
