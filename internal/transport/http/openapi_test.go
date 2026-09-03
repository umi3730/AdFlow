package httptransport

import (
	"os"
	"testing"

	"github.com/goccy/go-yaml"
)

func TestOpenAPIContractParses(t *testing.T) {
	content, err := os.ReadFile("../../../docs/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(content, &document); err != nil {
		t.Fatalf("invalid OpenAPI YAML: %v", err)
	}
	paths, ok := document["paths"].(map[string]any)
	if !ok {
		t.Fatal("OpenAPI paths are missing")
	}
	for _, path := range []string{"/v1/auth/login", "/v1/auth/me", "/v1/audit-logs"} {
		if _, exists := paths[path]; !exists {
			t.Fatalf("OpenAPI path %s is missing", path)
		}
	}
}
