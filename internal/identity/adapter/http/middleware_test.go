package httpadapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zhanghaiyang/adflow/internal/identity/domain"
	httptransport "github.com/zhanghaiyang/adflow/internal/transport/http"
)

type fakeAccess struct{ principal domain.Principal }

func (f fakeAccess) Authenticate(context.Context, string) (domain.Principal, error) {
	return f.principal, nil
}
func (f fakeAccess) Authorize(principal domain.Principal, method, route string) bool {
	return principal.Role == domain.RoleAdmin || method == http.MethodGet
}

func TestAuthenticationRequiresBearerToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httptransport.RequestID(), Authentication(true, fakeAccess{}), Authorization(fakeAccess{}))
	router.GET("/protected", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/protected", nil))
	if recorder.Code != http.StatusUnauthorized || recorder.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("status=%d headers=%v body=%s", recorder.Code, recorder.Header(), recorder.Body.String())
	}
}

func TestAuthenticationSetsPrincipal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	principal := domain.Principal{UserID: "1", Username: "admin", Role: domain.RoleAdmin}
	access := fakeAccess{principal: principal}
	router := gin.New()
	router.Use(httptransport.RequestID(), Authentication(true, access), Authorization(access))
	router.POST("/protected", func(c *gin.Context) {
		actual, ok := PrincipalFrom(c)
		if !ok || actual != principal {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodPost, "/protected", nil)
	request.Header.Set("Authorization", "Bearer valid-token")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestDisabledAuthenticationInjectsLocalAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httptransport.RequestID(), Authentication(false, fakeAccess{}), Authorization(fakeAccess{}))
	router.POST("/protected", func(c *gin.Context) {
		principal, _ := PrincipalFrom(c)
		c.String(http.StatusOK, string(principal.Role))
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/protected", nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "admin" {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
