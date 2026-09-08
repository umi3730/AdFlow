package httpadapter_test

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	campaignhttp "github.com/zhanghaiyang/adflow/internal/campaign/adapter/http"
	"github.com/zhanghaiyang/adflow/internal/campaign/adapter/memory"
	"github.com/zhanghaiyang/adflow/internal/campaign/application"
	decisionhttp "github.com/zhanghaiyang/adflow/internal/decision/adapter/http"
	decisionmemory "github.com/zhanghaiyang/adflow/internal/decision/adapter/memory"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDeleteHTTPContracts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repository := memory.NewRepository()
	router := gin.New()
	group := router.Group("/v1")
	campaignhttp.NewHandler(application.NewService(repository, nil), application.NewCreativeService(repository, repository)).RegisterRoutes(group)
	decisionhttp.NewHandler(nil, decisionmemory.NewRuntime(), nil).RegisterRoutes(group)
	call := func(method, path, body string, status int) map[string]any {
		t.Helper()
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)
		if recorder.Code != status {
			t.Fatalf("%s %s: %d %s", method, path, recorder.Code, recorder.Body.String())
		}
		result := map[string]any{}
		if recorder.Body.Len() > 0 {
			if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
		}
		return result
	}
	now := time.Now().UTC()
	body := `{"name":"delete test","slotId":"banner","startAt":"` + now.Format(time.RFC3339) + `","endAt":"` + now.Add(time.Hour).Format(time.RFC3339) + `"}`
	id := call("POST", "/v1/campaigns", body, 201)["id"].(string)
	other := call("POST", "/v1/campaigns", body, 201)["id"].(string)
	path := "/v1/campaigns/" + id
	creative := call("POST", path+"/creatives", `{"title":"test banner","imageUrl":"https://example.com/a.png","landingUrl":"https://example.com"}`, 201)["id"].(string)
	cp := path + "/creatives/" + creative
	call("POST", cp+"/enable", "", 422)
	call("DELETE", "/v1/campaigns/"+other+"/creatives/"+creative, "", 404)
	call("DELETE", cp, "", 409)
	call("POST", cp+"/disable", "", 200)
	call("POST", "/v1/campaigns/"+other+"/creatives/"+creative+"/enable", "", 404)
	enabled := call("POST", cp+"/enable", "", 200)
	if enabled["status"] != "ACTIVE" || enabled["revision"].(float64) != 3 {
		t.Fatal(enabled)
	}
	active, _ := repository.ListActiveCreativeIDsByCampaigns(t.Context(), []string{id})
	if len(active[id]) != 1 {
		t.Fatal("re-enabled creative did not rejoin candidate pool")
	}
	call("POST", cp+"/enable", "", 422)
	call("POST", cp+"/disable", "", 200)
	active, _ = repository.ListActiveCreativeIDsByCampaigns(t.Context(), []string{id})
	if len(active[id]) != 0 {
		t.Fatal("disabled creative still in candidate pool")
	}
	call("DELETE", cp, "", 204)
	call("POST", cp+"/enable", "", 404)
	call("DELETE", cp, "", 404)
	if len(call("GET", path+"/creatives", "", 200)["items"].([]any)) != 0 {
		t.Fatal("deleted creative listed")
	}
	call("POST", path+"/publish", `{"targeting":{"all":[{"tag":"anime"}]},"dailyBudgetFen":100,"impressionCostFen":1,"frequencyLimit":1}`, 200)
	call("DELETE", path, "", 409)
	call("POST", path+"/pause", "", 200)
	call("DELETE", path, "", 204)
	call("GET", path, "", 404)
	call("POST", path+"/resume", "", 404)
	call("DELETE", path, "", 404)
	if len(call("GET", "/v1/campaigns", "", 200)["items"].([]any)) != 1 {
		t.Fatal("deleted campaign listed")
	}
	otherPath := "/v1/campaigns/" + other
	child := call("POST", otherPath+"/creatives", `{"title":"test banner","imageUrl":"https://example.com/a.png","landingUrl":"https://example.com"}`, 201)["id"].(string)
	call("POST", otherPath+"/creatives/"+child+"/disable", "", 200)
	call("DELETE", otherPath, "", 204)
	call("POST", otherPath+"/creatives/"+child+"/enable", "", 404)
	call("PUT", "/v1/profiles/test-user", `{"tags":["anime"],"fields":{"age":"25"}}`, 204)
	call("DELETE", "/v1/profiles/test-user", "", 204)
	call("DELETE", "/v1/profiles/test-user", "", 204)
	call("GET", "/v1/profiles/test-user", "", 404)
	if call("GET", "/v1/profiles", "", 200)["total"].(float64) != 0 {
		t.Fatal("deleted profile listed")
	}
}
