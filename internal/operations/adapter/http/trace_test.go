package httpadapter

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	decisiondomain "github.com/zhanghaiyang/adflow/internal/decision/domain"
	"github.com/zhanghaiyang/adflow/internal/event/domain"
	"github.com/zhanghaiyang/adflow/internal/operations/application"
	"net/http"
	"net/http/httptest"
	"testing"
)

type inspectedID struct{ id string }

func (s *inspectedID) PeekDecision(_ context.Context, id string) (decisiondomain.Result, bool, error) {
	s.id = id
	return decisiondomain.Result{RequestID: id}, true, nil
}
func (s *inspectedID) ReadRequestState(context.Context, string) (domain.RequestState, error) {
	return domain.RequestState{}, nil
}
func TestTraceResolvesTemporaryRequestWithoutRunningDecision(t *testing.T) {
	gin.SetMode(gin.TestMode)
	source := &inspectedID{}
	router := gin.New()
	NewHandler(application.NewService(nil, nil), application.NewTraceService(source, source, "sync")).RegisterRoutes(router.Group("/v1"))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/operations/request-trace?requestId=client&simulationRunId=run-1&simulationUserId=user-0001", nil))
	_, expected := decisiondomain.SimulationIdentity("run-1", "user-0001", "client")
	var trace application.RequestTrace
	if err := json.Unmarshal(response.Body.Bytes(), &trace); err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 || source.id != expected || trace.RequestID != expected {
		t.Fatalf("status=%d trace=%+v lookedUp=%s", response.Code, trace, source.id)
	}
	for _, query := range []string{"requestId=", "requestId=r&simulationRunId=run", "requestId=r&simulationRunId=run&simulationUserId=user-0101"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/operations/request-trace?"+query, nil))
		if response.Code != 400 {
			t.Fatalf("query=%s status=%d", query, response.Code)
		}
	}
}
