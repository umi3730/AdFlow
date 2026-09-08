package httpadapter

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/umi3730/adflow/internal/event/domain"
	"github.com/umi3730/adflow/internal/operations/application"
)

type configuredOutbox struct{ domain.OperationsStore }

func TestRuntimeModeReportsWiringNotZeroMetrics(t *testing.T) {
	for _, tc := range []struct {
		name    string
		store   domain.OperationsStore
		enabled bool
	}{
		{"sync", nil, false}, {"kafka", configuredOutbox{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			router := gin.New()
			NewHandler(application.NewService(tc.store, nil)).RegisterRoutes(router.Group("/v1"))
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/operations/mode", nil))
			var got application.RuntimeMode
			if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != 200 || got.EventTransport != tc.name || got.OutboxEnabled != tc.enabled {
				t.Fatalf("unexpected mode: %+v", got)
			}
		})
	}
}
