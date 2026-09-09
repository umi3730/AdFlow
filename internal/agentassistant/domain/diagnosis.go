package domain

import (
	"context"
	"errors"
	"strings"
	"time"
)

var ErrInvalidDiagnosis = errors.New("投放诊断参数或模型结果无效")

type Evidence struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Detail     string `json:"detail"`
	Suggestion string `json:"suggestion"`
}
type DiagnosisContext struct {
	Question string     `json:"question"`
	Evidence []Evidence `json:"evidence"`
}
type Recommendation struct {
	Title       string   `json:"title"`
	Action      string   `json:"action"`
	EvidenceIDs []string `json:"evidenceIds"`
}
type Diagnosis struct {
	Summary           string           `json:"summary"`
	Recommendations   []Recommendation `json:"recommendations"`
	Evidence          []Evidence       `json:"evidence"`
	Provider          string           `json:"provider"`
	Model             string           `json:"model"`
	PromptVersion     string           `json:"promptVersion"`
	GeneratedAt       time.Time        `json:"generatedAt"`
	Fallback          bool             `json:"fallback"`
	InputTokens       int64            `json:"inputTokens,omitempty"`
	OutputTokens      int64            `json:"outputTokens,omitempty"`
	TotalTokens       int64            `json:"totalTokens,omitempty"`
	ProviderRequestID string           `json:"providerRequestId,omitempty"`
}
type DiagnosticProvider interface {
	Diagnose(context.Context, DiagnosisContext) (Diagnosis, error)
}

func ValidateDiagnosis(d Diagnosis, c DiagnosisContext) error {
	if strings.TrimSpace(d.Summary) == "" || len([]rune(d.Summary)) > 1500 || len(d.Recommendations) < 1 || len(d.Recommendations) > 6 {
		return ErrInvalidDiagnosis
	}
	ids := map[string]bool{}
	for _, e := range c.Evidence {
		ids[e.ID] = true
	}
	for _, r := range d.Recommendations {
		if strings.TrimSpace(r.Title) == "" || len([]rune(r.Title)) > 100 || strings.TrimSpace(r.Action) == "" || len([]rune(r.Action)) > 800 || len(r.EvidenceIDs) == 0 || len(r.EvidenceIDs) > 10 {
			return ErrInvalidDiagnosis
		}
		for _, id := range r.EvidenceIDs {
			if !ids[id] {
				return ErrInvalidDiagnosis
			}
		}
	}
	return nil
}
