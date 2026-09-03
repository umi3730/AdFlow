package observability

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	eventdomain "github.com/zhanghaiyang/adflow/internal/event/domain"
)

func TestMetricsExposeHTTPDecisionAndEventSeries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	metrics := New()
	router := gin.New()
	router.Use(metrics.Middleware())
	router.GET("/ok", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ok", nil))
	metrics.ObserveDecision(true, "matched", time.Millisecond)
	metrics.ObserveDecisionAdmission("accepted", time.Millisecond)
	metrics.AddDecisionInFlight(1)
	metrics.AddDecisionInFlight(-1)
	metrics.ObserveDecisionTimeout()
	metrics.ObserveAgentGeneration("openai-compatible", "test-model", "success", time.Second, 10, 5)
	metrics.SetAgentCircuitOpen(true)
	metrics.SetAgentCircuitOpen(false)
	metrics.ObserveEvent("impression", true)
	metrics.SetOutboxDepth(eventdomain.OutboxStats{Pending: 2})
	metrics.ObserveOutboxResult("published")
	metrics.SetKafkaConsumerLag("adflow.ad-events.v1", 0, 3)

	metricsRecorder := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(metricsRecorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := metricsRecorder.Body.String()
	for _, name := range []string{"adflow_http_requests_total", "adflow_decision_results_total", "adflow_decision_admission_total", "adflow_decision_queue_duration_seconds", "adflow_decision_in_flight", "adflow_decision_execution_timeouts_total", "adflow_agent_generations_total", "adflow_agent_generation_duration_seconds", "adflow_agent_tokens_total", "adflow_agent_circuit_open", "adflow_event_records_total", "adflow_outbox_rows", "adflow_kafka_consumer_lag", "go_goroutines"} {
		if !strings.Contains(body, name) {
			t.Fatalf("missing metric %s", name)
		}
	}
}
