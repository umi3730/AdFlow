package openaiadapter

import (
	"encoding/json"
	"github.com/umi3730/adflow/internal/agentassistant/domain"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDiagnosisBothProtocolsAndEvidenceValidation(t *testing.T) {
	for _, style := range []string{APIStyleResponses, APIStyleChatCompletions} {
		t.Run(style, func(t *testing.T) {
			var fabricated atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				evidenceID := "report"
				if fabricated.Load() {
					evidenceID = "fabricated"
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				serialized, _ := json.Marshal(body)
				if !strings.Contains(string(serialized), "untrusted") || !strings.Contains(string(serialized), "evidenceIds") {
					t.Error("missing diagnostic contract")
				}
				narrative := `{"summary":"已有曝光，建议检查点击回传。","recommendations":[{"title":"检查回传","action":"查看事件处理状态。","evidenceIds":["` + evidenceID + `"]}]}`
				if style == APIStyleResponses {
					if r.URL.Path != "/responses" {
						t.Error(r.URL.Path)
					}
					json.NewEncoder(w).Encode(map[string]any{"status": "completed", "model": "test", "output": []any{map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": narrative}}}}})
				} else {
					if r.URL.Path != "/chat/completions" {
						t.Error(r.URL.Path)
					}
					json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": narrative}}}})
				}
			}))
			defer server.Close()
			p, err := NewProvider(providerConfig(server.URL, style))
			if err != nil {
				t.Error(err)
				return
			}
			input := domain.DiagnosisContext{Question: "分析投放", Evidence: []domain.Evidence{{ID: "report", Detail: "曝光100次"}}}
			d, err := p.Diagnose(t.Context(), input)
			if err != nil || d.Provider != "openai-compatible" || len(d.Recommendations) != 1 {
				t.Fatal(d, err)
			}
			fabricated.Store(true)
			if _, err = p.Diagnose(t.Context(), input); err == nil {
				t.Fatal("accepted fabricated evidence")
			}
		})
	}
}
