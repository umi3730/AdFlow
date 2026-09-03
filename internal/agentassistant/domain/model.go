package domain

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidPrompt = errors.New("prompt must contain 5 to 2000 characters")
	ErrInvalidDraft  = errors.New("generated rule draft is invalid")
)

type Condition struct {
	Tag   string `json:"tag,omitempty"`
	Field string `json:"field,omitempty"`
	Op    string `json:"op,omitempty"`
	Value string `json:"value,omitempty"`
}

type TargetingRule struct {
	All  []Condition `json:"all,omitempty"`
	Any  []Condition `json:"any,omitempty"`
	None []Condition `json:"none,omitempty"`
}

type Draft struct {
	Targeting         TargetingRule `json:"targeting"`
	DailyBudgetFen    int64         `json:"dailyBudgetFen"`
	ImpressionCostFen int64         `json:"impressionCostFen"`
	FrequencyLimit    uint32        `json:"frequencyLimit"`
	Explanation       string        `json:"explanation"`
	Warnings          []string      `json:"warnings"`
	Provider          string        `json:"provider"`
	Model             string        `json:"model"`
	PromptVersion     string        `json:"promptVersion"`
	GeneratedAt       time.Time     `json:"generatedAt"`
}

type Provider interface {
	Generate(context.Context, string) (Draft, error)
}
