package httptransport

import (
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCORSAllowsDeletePreflight(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CORS("test"))
	request := httptest.NewRequest("OPTIONS", "/v1/campaigns/example", nil)
	request.Header.Set("Origin", "http://127.0.0.1:3000")
	request.Header.Set("Access-Control-Request-Method", "DELETE")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 204 || !strings.Contains(response.Header().Get("Access-Control-Allow-Methods"), "DELETE") {
		t.Fatal("DELETE preflight rejected")
	}
}
