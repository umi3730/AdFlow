package httptransport

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zhanghaiyang/adflow/internal/health"
	"github.com/zhanghaiyang/adflow/internal/observability"
)

type fakeChecker struct{ err error }

func (f fakeChecker) PingContext(context.Context) error { return f.err }

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestHealthEndpoints(t *testing.T) {
	service := health.NewService(time.Second, map[string]health.Checker{"dependency": fakeChecker{}})
	router := NewRouter("test", testLogger(), service, observability.New())

	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if recorder.Header().Get(RequestIDHeader) == "" {
		t.Fatal("expected response request id")
	}
}

func TestReadyReportsDependencyFailure(t *testing.T) {
	service := health.NewService(time.Second, map[string]health.Checker{"mysql": fakeChecker{err: errors.New("down")}})
	router := NewRouter("test", testLogger(), service, observability.New())

	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}

func TestRequestIDPreservesCallerValue(t *testing.T) {
	service := health.NewService(time.Second, map[string]health.Checker{})
	router := NewRouter("test", testLogger(), service, observability.New())
	request := httptest.NewRequest(http.MethodGet, "/livez", nil)
	request.Header.Set(RequestIDHeader, "test-request-id")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if got := recorder.Header().Get(RequestIDHeader); got != "test-request-id" {
		t.Fatalf("request id = %q", got)
	}
}

func TestBindJSONReturnsFieldErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID())
	type payload struct {
		Name string `json:"name" binding:"required,min=3"`
	}
	router.POST("/payload", func(c *gin.Context) {
		var input payload
		if !BindJSON(c, &input) {
			return
		}
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodPost, "/payload", strings.NewReader(`{"name":""}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if !strings.Contains(recorder.Body.String(), `"code":"validation_failed"`) || !strings.Contains(recorder.Body.String(), `"field":"Name"`) {
		t.Fatalf("unexpected response: %s", recorder.Body.String())
	}
}

func TestRecoveryReturnsStandardError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID(), Recovery(testLogger()))
	router.GET("/panic", func(*gin.Context) { panic("boom") })

	request := httptest.NewRequest(http.MethodGet, "/panic", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}

func TestLocalCORSAllowsAdministrationUI(t *testing.T) {
	service := health.NewService(time.Second, map[string]health.Checker{})
	router := NewRouter("test", testLogger(), service, observability.New())
	request := httptest.NewRequest(http.MethodOptions, "/v1/campaigns", nil)
	request.Header.Set("Origin", "http://localhost:3000")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Fatalf("allow origin = %q", got)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "Authorization") {
		t.Fatalf("allow headers = %q", got)
	}
}
