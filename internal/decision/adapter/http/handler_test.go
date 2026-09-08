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

	"github.com/umi3730/adflow/internal/campaign/adapter/memory"
	campaignapp "github.com/umi3730/adflow/internal/campaign/application"
	"github.com/umi3730/adflow/internal/campaign/domain"
	decisioncampaign "github.com/umi3730/adflow/internal/decision/adapter/campaign"
	httpadapter "github.com/umi3730/adflow/internal/decision/adapter/http"
	decisionmemory "github.com/umi3730/adflow/internal/decision/adapter/memory"
	decisionapp "github.com/umi3730/adflow/internal/decision/application"
	decisiondomain "github.com/umi3730/adflow/internal/decision/domain"
	"github.com/umi3730/adflow/internal/health"
	"github.com/umi3730/adflow/internal/observability"
	httptransport "github.com/umi3730/adflow/internal/transport/http"
)

type okChecker struct{}

func (okChecker) PingContext(context.Context) error { return nil }

type errorDecisionEngine struct{ err error }

func (e errorDecisionEngine) Decide(context.Context, decisiondomain.Request) (decisiondomain.Result, error) {
	return decisiondomain.Result{}, e.err
}

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
	conflict := request(t, router, http.MethodPost, "/v1/decisions", `{"requestId":"request-1","userId":"other","slotId":"game-home-banner"}`)
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), "request_id_conflict") {
		t.Fatalf("conflict=%d %s", conflict.Code, conflict.Body.String())
	}
	second := request(t, router, http.MethodPost, "/v1/decisions", `{"requestId":"request-2","userId":"user-1","slotId":"game-home-banner"}`)
	assertDecision(t, second, false, "frequency_capped")
}

func TestProfileReadAndExplanationHTTP(t *testing.T) {
	runtime := decisionmemory.NewRuntime()
	_ = runtime.PutProfile(t.Context(), decisiondomain.NewProfile("user-1", []string{"strategy_game", "anime"}, map[string]string{"device": "android", "custom": "kept"}))
	repository := memory.NewRepository()
	source := decisioncampaign.NewProvider(repository, repository)
	handler := httpadapter.NewHandler(errorDecisionEngine{}, runtime, nil)
	explainer := httpadapter.NewExplanationHandler(decisionapp.NewExplanationService(runtime, source))
	router := httptransport.NewRouter("test", slog.New(slog.NewTextHandler(io.Discard, nil)), health.NewService(time.Second, map[string]health.Checker{"test": okChecker{}}), observability.New(), handler, explainer)
	listed := request(t, router, http.MethodGet, "/v1/profiles?tag=anime&device=android&limit=20", "")
	if listed.Code != 200 || !strings.Contains(listed.Body.String(), `"total":1`) || !strings.Contains(listed.Body.String(), `"custom":"kept"`) {
		t.Fatalf("status=%d body=%s", listed.Code, listed.Body.String())
	}
	if got := request(t, router, http.MethodGet, "/v1/profiles/missing", ""); got.Code != 404 {
		t.Fatalf("missing status=%d", got.Code)
	}
	if got := request(t, router, http.MethodGet, "/v1/profiles?limit=500", ""); got.Code != 400 {
		t.Fatalf("invalid pagination status=%d", got.Code)
	}
	explained := request(t, router, http.MethodPost, "/v1/decisions/explain", `{"userId":"user-1","slotId":"slot-1"}`)
	if explained.Code != 200 || !strings.Contains(explained.Body.String(), `"scope":"current_targeting_only"`) || !strings.Contains(explained.Body.String(), `"candidates":[]`) {
		t.Fatalf("status=%d body=%s", explained.Code, explained.Body.String())
	}
}

func TestDecisionAdmissionErrorsUseExplicitHTTPStatuses(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "rate limited", err: decisiondomain.ErrRateLimited, status: http.StatusTooManyRequests, code: "decision_rate_limited"},
		{name: "backpressure", err: decisiondomain.ErrBackpressure, status: http.StatusServiceUnavailable, code: "decision_backpressure"},
		{name: "backpressure unavailable", err: decisiondomain.ErrBackpressureUnavailable, status: http.StatusServiceUnavailable, code: "decision_backpressure_unavailable"},
		{name: "overloaded", err: decisiondomain.ErrOverloaded, status: http.StatusServiceUnavailable, code: "decision_overloaded"},
		{name: "limiter unavailable", err: decisiondomain.ErrAdmissionUnavailable, status: http.StatusServiceUnavailable, code: "decision_admission_unavailable"},
		{name: "timed out", err: decisiondomain.ErrDecisionTimeout, status: http.StatusGatewayTimeout, code: "decision_timeout"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := httpadapter.NewHandler(errorDecisionEngine{err: tc.err}, nil, nil)
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			healthService := health.NewService(time.Second, map[string]health.Checker{"test": okChecker{}})
			router := httptransport.NewRouter("test", logger, healthService, observability.New(), handler)
			response := request(t, router, http.MethodPost, "/v1/decisions", `{"requestId":"request-1","userId":"user-1","slotId":"slot-1"}`)
			if response.Code != tc.status || !strings.Contains(response.Body.String(), `"code":"`+tc.code+`"`) {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if strings.HasPrefix(tc.code, "decision_backpressure") && response.Header().Get("Retry-After") != "1" {
				t.Fatal("missing retry guidance")
			}
		})
	}
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
