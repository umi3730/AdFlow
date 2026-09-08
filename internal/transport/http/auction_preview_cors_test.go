package httptransport

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsolatedPreviewCORSIsLimitedToDevelopmentLoopback(t *testing.T) {
	for _, row := range []struct {
		environment, origin string
		allowed             bool
	}{{"local", "http://127.0.0.1:3001", true}, {"test", "http://localhost:3001", true}, {"production", "http://127.0.0.1:3001", false}, {"local", "http://example.com:3001", false}} {
		router := gin.New()
		router.Use(CORS(row.environment))
		request := httptest.NewRequest(http.MethodOptions, "/v1/decisions", nil)
		request.Header.Set("Origin", row.origin)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if (recorder.Code == http.StatusNoContent) != row.allowed {
			t.Fatalf("%+v status=%d", row, recorder.Code)
		}
	}
}
