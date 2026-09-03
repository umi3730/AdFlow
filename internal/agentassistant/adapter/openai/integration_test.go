package openaiadapter_test

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
	"time"

	openaiadapter "github.com/zhanghaiyang/adflow/internal/agentassistant/adapter/openai"
	"github.com/zhanghaiyang/adflow/internal/agentassistant/application"
)

type evaluationCase struct {
	Name              string   `json:"name"`
	Prompt            string   `json:"prompt"`
	RequiredTags      []string `json:"requiredTags"`
	ExcludedTags      []string `json:"excludedTags"`
	MaxDailyBudgetFen int64    `json:"maxDailyBudgetFen"`
	MaxFrequencyLimit uint32   `json:"maxFrequencyLimit"`
}

func TestLiveProviderEvaluation(t *testing.T) {
	if os.Getenv("ADFLOW_AGENT_RUN_LIVE_EVALS") != "true" {
		t.Skip("set ADFLOW_AGENT_RUN_LIVE_EVALS=true to opt into billable live Agent evaluations")
	}
	apiKey := os.Getenv("ADFLOW_AGENT_API_KEY")
	model := os.Getenv("ADFLOW_AGENT_MODEL")
	if apiKey == "" || model == "" {
		t.Skip("set ADFLOW_AGENT_API_KEY and ADFLOW_AGENT_MODEL to run live Agent evaluations")
	}
	baseURL := environmentOr("ADFLOW_AGENT_BASE_URL", "https://api.openai.com/v1")
	style := environmentOr("ADFLOW_AGENT_API_STYLE", openaiadapter.APIStyleResponses)
	provider, err := openaiadapter.NewProvider(openaiadapter.ProviderConfig{
		BaseURL: baseURL, APIKey: apiKey, Model: model, APIStyle: style, Timeout: 15 * time.Second, MaxRetries: 2,
		MaxDailyBudgetFen: 10_000_000, MaxImpressionCostFen: 100_000, MaxConditions: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewServiceWithPolicy(provider, application.DefaultPolicy())
	content, err := os.ReadFile("../../../../tests/evals/agent-rule-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []evaluationCase
	if err := json.Unmarshal(content, &cases); err != nil {
		t.Fatal(err)
	}
	for _, evaluation := range cases {
		t.Run(evaluation.Name, func(t *testing.T) {
			draft, err := service.Generate(t.Context(), evaluation.Prompt)
			if err != nil {
				t.Fatal(err)
			}
			if draft.DailyBudgetFen > evaluation.MaxDailyBudgetFen || draft.FrequencyLimit > evaluation.MaxFrequencyLimit {
				t.Fatalf("unsafe limits: budget=%d frequency=%d", draft.DailyBudgetFen, draft.FrequencyLimit)
			}
			allTags := make([]string, 0, len(draft.Targeting.All)+len(draft.Targeting.Any))
			for _, condition := range append(draft.Targeting.All, draft.Targeting.Any...) {
				if condition.Tag != "" {
					allTags = append(allTags, condition.Tag)
				}
			}
			noneTags := make([]string, 0, len(draft.Targeting.None))
			for _, condition := range draft.Targeting.None {
				if condition.Tag != "" {
					noneTags = append(noneTags, condition.Tag)
				}
			}
			for _, tag := range evaluation.RequiredTags {
				if !slices.Contains(allTags, tag) {
					t.Errorf("required tag %q missing from %v", tag, allTags)
				}
			}
			for _, tag := range evaluation.ExcludedTags {
				if !slices.Contains(noneTags, tag) {
					t.Errorf("excluded tag %q missing from %v", tag, noneTags)
				}
			}
		})
	}
}

func environmentOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
