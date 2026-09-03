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

	"github.com/zhanghaiyang/adflow/internal/campaign/adapter/memory"
	campaignapp "github.com/zhanghaiyang/adflow/internal/campaign/application"
	"github.com/zhanghaiyang/adflow/internal/campaign/domain"
	decisioncampaign "github.com/zhanghaiyang/adflow/internal/decision/adapter/campaign"
	httpadapter "github.com/zhanghaiyang/adflow/internal/decision/adapter/http"
	decisionmemory "github.com/zhanghaiyang/adflow/internal/decision/adapter/memory"
	decisionapp "github.com/zhanghaiyang/adflow/internal/decision/application"
	"github.com/zhanghaiyang/adflow/internal/health"
	"github.com/zhanghaiyang/adflow/internal/observability"
	httptransport "github.com/zhanghaiyang/adflow/internal/transport/http"
)

type okChecker struct{}

func (okChecker) PingContext(context.Context) error { return nil }

func TestDecisionHTTPFlow(t *testing.T) {
	ctx := context.Background()
	repository := memory.NewRepository()
	campaignService := campaignapp.NewService(repository, nil)
	creativeService := campaignapp.NewCreativeService(repository, repository)
	now := time.Now().UTC()
	campaign, err := campaignService.Create(ctx, campaignapp.CreateCommand{
		Name: "Strategy Campaign", SlotID: "game-home-banner", StartAt: now.Add(-time.Hour), EndAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = campaignService.Publish(ctx, campaignapp.PublishCommand{
		CampaignID: campaign.ID(), All: []domain.Condition{{Tag: "anime"}},
		DailyBudgetFen: 1000, ImpressionCostFen: 100, FrequencyLimit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = creativeService.Create(ctx, campaignapp.CreateCreativeCommand{
		CampaignID: campaign.ID(), Title: "Banner", ImageURL: "https://example.com/banner.png", LandingURL: "https://example.com/game",
	})
	if err != nil {
		t.Fatal(err)
	}

	runtime := decisionmemory.NewRuntime()
	service := decisionapp.NewService(decisioncampaign.NewProvider(repository, repository), runtime, runtime, runtime, runtime)
	handler := httpadapter.NewHandler(service, runtime, nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	healthService := health.NewService(time.Second, map[string]health.Checker{"test": okChecker{}})
	router := httptransport.NewRouter("test", logger, healthService, observability.New(), handler)

	profileRecorder := request(t, router, http.MethodPut, "/v1/profiles/user-1", `{"tags":["anime"],"fields":{"device":"android"}}`)
	if profileRecorder.Code != http.StatusNoContent {
		t.Fatalf("profile status = %d body=%s", profileRecorder.Code, profileRecorder.Body.String())
	}

	first := request(t, router, http.MethodPost, "/v1/decisions", `{"requestId":"request-1","userId":"user-1","slotId":"game-home-banner"}`)
	assertDecision(t, first, true, "matched")
	second := request(t, router, http.MethodPost, "/v1/decisions", `{"requestId":"request-2","userId":"user-1","slotId":"game-home-banner"}`)
	assertDecision(t, second, false, "frequency_capped")
}

func request(t *testing.T, handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

func assertDecision(t *testing.T, recorder *httptest.ResponseRecorder, matched bool, reason string) {
	t.Helper()
	if recorder.Code != http.StatusOK {
		t.Fatalf("decision status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Matched bool   `json:"matched"`
		Reason  string `json:"reason"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Matched != matched || response.Reason != reason {
		t.Fatalf("unexpected decision: %+v", response)
	}
}
