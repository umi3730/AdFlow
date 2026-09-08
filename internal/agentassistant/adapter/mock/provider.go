package mock

import (
	"context"
	"strings"

	"github.com/umi3730/adflow/internal/agentassistant/domain"
)

type Provider struct{}

func NewProvider() *Provider { return &Provider{} }

func (*Provider) Generate(_ context.Context, prompt string) (domain.Draft, error) {
	lower := strings.ToLower(prompt)
	all := make([]domain.Condition, 0, 4)
	none := make([]domain.Condition, 0, 1)
	anyOf := make([]domain.Condition, 0, 2)
	tech := strings.Contains(lower, "数码") || strings.Contains(lower, "technology")
	shopping := strings.Contains(lower, "购物") || strings.Contains(lower, "shopping")
	gaming := strings.Contains(lower, "游戏兴趣") || strings.Contains(lower, "对游戏") || strings.Contains(lower, "或游戏") || strings.Contains(lower, "gaming")
	interests := make([]domain.Condition, 0, 2)
	if tech {
		interests = append(interests, domain.Condition{Tag: "tech_interest"})
	}
	if shopping {
		interests = append(interests, domain.Condition{Tag: "shopping_interest"})
	}
	if gaming {
		interests = append(interests, domain.Condition{Tag: "gaming_interest"})
	}
	if len(interests) > 1 && (strings.Contains(lower, "或") || strings.Contains(lower, " or ")) {
		anyOf = interests
	} else {
		all = append(all, interests...)
	}
	if strings.Contains(lower, "新用户") || strings.Contains(lower, "new user") {
		if strings.Contains(lower, "排除新用户") || strings.Contains(lower, "非新用户") {
			none = append(none, domain.Condition{Tag: "new_user"})
		} else {
			all = append(all, domain.Condition{Tag: "new_user"})
		}
	}
	if strings.Contains(lower, "付费用户") || strings.Contains(lower, "paying user") {
		if strings.Contains(lower, "排除付费用户") || strings.Contains(lower, "未付费用户") || strings.Contains(lower, "非付费用户") {
			none = append(none, domain.Condition{Tag: "paying_user"})
		} else {
			all = append(all, domain.Condition{Tag: "paying_user"})
		}
	}
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
	if len(all)+len(none)+len(anyOf) == 0 {
		all = append(all, domain.Condition{Tag: "general_audience"})
	}
	return domain.Draft{
		Targeting:      domain.TargetingRule{All: all, Any: anyOf, None: none},
		DailyBudgetFen: 100000, ImpressionCostFen: 100, FrequencyLimit: 3,
		Explanation: "根据自然语言中的兴趣、活跃度和设备关键词生成结构化定向草稿。",
		Warnings:    []string{"当前为本地关键词解析，年龄、会员等级和来源渠道未解析，请手动补充；预算、频控也需人工核对。"},
		Provider:    "local-mock", Model: "deterministic-rule-parser", PromptVersion: "rule-draft-v1",
	}, nil
}
