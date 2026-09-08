package httpadapter

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/umi3730/adflow/internal/identity/adapter/jwt"
	"github.com/umi3730/adflow/internal/identity/adapter/memory"
	"github.com/umi3730/adflow/internal/identity/adapter/password"
	"github.com/umi3730/adflow/internal/identity/application"
	"github.com/umi3730/adflow/internal/identity/domain"
)

func TestRegistrationHTTPAndAdminAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	users, _ := memory.NewUserStore("", "test", true)
	tokens, _ := jwtadapter.NewManager("01234567890123456789012345678901", "test", time.Hour)
	s := application.NewService(users, password.Bcrypt{}, tokens)
	s.EnableRegistration(password.Bcrypt{})
	h := NewHandler(s, true)
	h.SetDemoPrefill(true)
	router := gin.New()
	group := router.Group("/v1")
	h.RegisterRoutes(group)
	group.POST("/campaigns/:id/publish", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	post := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/auth/register", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(r, req)
		return r
	}
	opts := httptest.NewRecorder()
	router.ServeHTTP(opts, httptest.NewRequest("GET", "/v1/auth/options", nil))
	if opts.Code != 200 || !strings.Contains(opts.Body.String(), `"demoLoginPrefill":true`) {
		t.Fatal(opts.Code, opts.Body.String())
	}
	r := post(`{"username":"new-admin","password":"demo-password"}`)
	if r.Code != 201 {
		t.Fatal(r.Code, r.Body.String())
	}
	var token domain.IssuedToken
	if err := json.Unmarshal(r.Body.Bytes(), &token); err != nil {
		t.Fatal(err)
	}
	admin := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/campaigns/example/publish", nil)
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	router.ServeHTTP(admin, req)
	if admin.Code != 204 {
		t.Fatal("registered user cannot publish", admin.Code)
	}
	if r = post(`{"username":"NEW-ADMIN","password":"demo-password"}`); r.Code != 409 {
		t.Fatal(r.Code, r.Body.String())
	}
	if r = post(`{"username":"bad name","password":"demo-password"}`); r.Code != 422 {
		t.Fatal(r.Code, r.Body.String())
	}
	if r = post(`{"username":"other-user","password":"` + strings.Repeat("x", 73) + `"}`); r.Code != 422 {
		t.Fatal(r.Code, r.Body.String())
	}
}

func TestRegistrationDisabled(t *testing.T) {
	s := application.NewService(nil, nil, nil)
	r := gin.New()
	NewHandler(s, true).RegisterRoutes(r.Group("/v1"))
	res := httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest("POST", "/v1/auth/register", nil))
	if res.Code != 403 {
		t.Fatal(res.Code)
	}
}
