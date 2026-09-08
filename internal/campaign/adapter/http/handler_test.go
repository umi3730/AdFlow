package httpadapter_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	httpadapter "github.com/umi3730/adflow/internal/campaign/adapter/http"
	"github.com/umi3730/adflow/internal/campaign/adapter/memory"
	"github.com/umi3730/adflow/internal/campaign/application"
	"github.com/umi3730/adflow/internal/health"
	"github.com/umi3730/adflow/internal/observability"
	httptransport "github.com/umi3730/adflow/internal/transport/http"
)

type okChecker struct{}

func (okChecker) PingContext(context.Context) error { return nil }

func TestCampaignHTTPLifecycle(t *testing.T) {
	repository := memory.NewRepository()
	service := application.NewService(repository, nil)
	creativeService := application.NewCreativeService(repository, repository)
	handler := httpadapter.NewHandler(service, creativeService)
	healthService := health.NewService(time.Second, map[string]health.Checker{"test": okChecker{}})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := httptransport.NewRouter("test", logger, healthService, observability.New(), handler)

	start := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	end := time.Now().UTC().Add(25 * time.Hour).Format(time.RFC3339)
	createBody := `{"name":"Strategy Campaign","slotId":"game-home-banner","startAt":"` + start + `","endAt":"` + end + `"}`
	created := requestJSON(t, router, http.MethodPost, "/v1/campaigns", createBody, http.StatusCreated)
	id, ok := created["id"].(string)
	if !ok || id == "" {
		t.Fatalf("missing campaign id: %+v", created)
	}

	publishBody := `{"targeting":{"all":[{"tag":"anime"}]},"dailyBudgetFen":10000,"impressionCostFen":100,"frequencyLimit":3}`
	published := requestJSON(t, router, http.MethodPost, "/v1/campaigns/"+id+"/publish", publishBody, http.StatusOK)
	if published["status"] != "ACTIVE" {
		t.Fatalf("status = %v", published["status"])
	}

	paused := requestJSON(t, router, http.MethodPost, "/v1/campaigns/"+id+"/pause", "", http.StatusOK)
	if paused["status"] != "PAUSED" {
		t.Fatalf("status = %v", paused["status"])
	}

	resumed := requestJSON(t, router, http.MethodPost, "/v1/campaigns/"+id+"/resume", "", http.StatusOK)
	if resumed["status"] != "ACTIVE" {
		t.Fatalf("status = %v", resumed["status"])
	}

	creativeBody := `{"title":"Launch Banner","description":"Strategy game creative","imageUrl":"https://example.com/banner.png","landingUrl":"https://example.com/game"}`
	creative := requestJSON(t, router, http.MethodPost, "/v1/campaigns/"+id+"/creatives", creativeBody, http.StatusCreated)
	creativeID, ok := creative["id"].(string)
	if !ok || creativeID == "" {
		t.Fatalf("missing creative id: %+v", creative)
	}
	disabled := requestJSON(t, router, http.MethodPost, "/v1/campaigns/"+id+"/creatives/"+creativeID+"/disable", "", http.StatusOK)
	if disabled["status"] != "DISABLED" {
		t.Fatalf("creative status = %v", disabled["status"])
	}

	listed := requestJSON(t, router, http.MethodGet, "/v1/campaigns?status=ACTIVE&limit=10", "", http.StatusOK)
	items, ok := listed["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("unexpected campaign list: %+v", listed)
	}
}

func requestJSON(t *testing.T, handler http.Handler, method, path, body string, wantStatus int) map[string]any {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != wantStatus {
		t.Fatalf("%s %s status = %d, want %d; body=%s", method, path, recorder.Code, wantStatus, recorder.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return response
}
