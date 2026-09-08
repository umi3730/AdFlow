package httpadapter

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/zhanghaiyang/adflow/internal/decision/adapter/memory"
	"github.com/zhanghaiyang/adflow/internal/decision/adapter/requestprofile"
	"github.com/zhanghaiyang/adflow/internal/decision/application"
	"github.com/zhanghaiyang/adflow/internal/decision/domain"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

type simulationCandidates struct{}

func (simulationCandidates) ActiveCandidates(context.Context, string, time.Time) ([]domain.Candidate, error) {
	return []domain.Candidate{{CampaignID: "campaign", CreativeIDs: []string{"creative"}, Targeting: domain.TargetingRule{All: []domain.Condition{{Tag: "gaming_interest"}}}, DailyBudgetFen: 100, ImpressionCostFen: 1, FrequencyLimit: 1}}, nil
}

func TestTemporaryProfilesNeverEnterCatalogAndNamespacesIsolateFrequency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := memory.NewRuntime()
	real := domain.NewProfile("user-0001", []string{"gaming_interest"}, map[string]string{"device": "ios", "score": "88"})
	_ = store.PutProfile(t.Context(), real)
	engine := application.NewService(simulationCandidates{}, requestprofile.New(store), store, store, store)
	router := gin.New()
	group := router.Group("/v1")
	NewHandler(engine, store, nil).RegisterRoutes(group)
	NewSimulationHandler(engine, nil, "test").RegisterRoutes(group)
	call := func(path, body, remote string, status int) map[string]any {
		t.Helper()
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		req.RemoteAddr = remote
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != status {
			t.Fatalf("%s: %d %s", path, res.Code, res.Body.String())
		}
		result := map[string]any{}
		_ = json.Unmarshal(res.Body.Bytes(), &result)
		return result
	}
	payload := func(run, request string) string {
		return `{"runId":"` + run + `","requestId":"` + request + `","slotId":"banner","profile":{"userId":"user-0001","tags":["gaming_interest"],"fields":{"device":"android","score":"40"}}}`
	}
	first := call("/v1/simulations/decisions", payload("run-a", "one"), "127.0.0.1:1234", 200)
	if first["matched"] != true {
		t.Fatal(first)
	}
	cached := call("/v1/simulations/decisions", payload("run-a", "one"), "127.0.0.1:1234", 200)
	if cached["requestId"] != first["requestId"] || cached["matched"] != true {
		t.Fatal("idempotency failed", cached)
	}
	conflict := call("/v1/simulations/decisions", strings.Replace(payload("run-a", "one"), `"android"`, `"ios"`, 1), "127.0.0.1:1234", 409)
	if conflict["error"].(map[string]any)["code"] != "request_id_conflict" {
		t.Fatal(conflict)
	}
	capped := call("/v1/simulations/decisions", payload("run-a", "two"), "127.0.0.1:1234", 200)
	if capped["reason"] != "frequency_capped" {
		t.Fatal(capped)
	}
	other := call("/v1/simulations/decisions", payload("run-b", "one"), "127.0.0.1:1234", 200)
	if other["matched"] != true || other["requestId"] == first["requestId"] {
		t.Fatal("run namespaces collided", other)
	}
	normal := call("/v1/decisions", `{"requestId":"real","userId":"user-0001","slotId":"banner"}`, "127.0.0.1:1234", 200)
	if normal["matched"] != true {
		t.Fatal("temporary user consumed real user's frequency", normal)
	}
	saved, _ := store.FindProfile(t.Context(), real.UserID)
	page, _ := store.ListProfiles(t.Context(), domain.ProfileFilter{Limit: 100})
	if !reflect.DeepEqual(real, saved) || page.Total != 1 {
		t.Fatal("formal profile store changed")
	}
	result, found, _ := store.FindDecision(t.Context(), first["requestId"].(string))
	if !found || result.UserID == real.UserID || len(result.UserID) > 128 {
		t.Fatal("invalid internal identity", result)
	}
	call("/v1/simulations/decisions", payload("run-c", "one"), "192.0.2.1:1234", 403)
	call("/v1/simulations/decisions", strings.ReplaceAll(payload("run-c", "one"), "user-0001", "user-1001"), "127.0.0.1:1234", 422)
	prod := gin.New()
	NewSimulationHandler(engine, nil, "production").RegisterRoutes(prod.Group("/v1"))
	res := httptest.NewRecorder()
	prod.ServeHTTP(res, httptest.NewRequest("POST", "/v1/simulations/decisions", nil))
	if res.Code != 404 {
		t.Fatal("simulation exposed in production")
	}
}
