package httpadapter

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	jwtadapter "github.com/umi3730/adflow/internal/identity/adapter/jwt"
	"github.com/umi3730/adflow/internal/identity/adapter/memory"
	"github.com/umi3730/adflow/internal/identity/adapter/password"
	"github.com/umi3730/adflow/internal/identity/application"
	"github.com/umi3730/adflow/internal/identity/domain"
	httptransport "github.com/umi3730/adflow/internal/transport/http"
)

func TestLoginAndRoleProtectedRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	users, err := memory.NewUserStore("", "test", true)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := jwtadapter.NewManager("01234567890123456789012345678901", "adflow-test", 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewService(users, password.Bcrypt{}, tokens)
	router := gin.New()
	router.Use(httptransport.RequestID())
	api := router.Group("/v1")
	NewHandler(service, true).RegisterRoutes(api)
	api.GET("/campaigns", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	api.POST("/campaigns/:id/publish", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/v1/campaigns", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status=%d", unauthorized.Code)
	}

	operatorToken := login(t, router, "operator", "adflow-operator")
	forbidden := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/campaigns/campaign-1/publish", nil)
	request.Header.Set("Authorization", "Bearer "+operatorToken)
	router.ServeHTTP(forbidden, request)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("operator publish status=%d body=%s", forbidden.Code, forbidden.Body.String())
	}

	adminToken := login(t, router, "admin", "adflow-admin")
	allowed := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/campaigns/campaign-1/publish", nil)
	request.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(allowed, request)
	if allowed.Code != http.StatusNoContent {
		t.Fatalf("admin publish status=%d body=%s", allowed.Code, allowed.Body.String())
	}

	viewerToken := login(t, router, "viewer", "adflow-viewer")
	read := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/v1/campaigns", nil)
	request.Header.Set("Authorization", "Bearer "+viewerToken)
	router.ServeHTTP(read, request)
	if read.Code != http.StatusNoContent {
		t.Fatalf("viewer read status=%d body=%s", read.Code, read.Body.String())
	}
}

func login(t *testing.T, router http.Handler, username, plainPassword string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": plainPassword})
	request := httptest.NewRequest(http.MethodPost, "/v1/auth/login", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response domain.IssuedToken
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response.AccessToken
}
