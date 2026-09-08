package httpadapter

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zhanghaiyang/adflow/internal/audit/adapter/memory"
	"github.com/zhanghaiyang/adflow/internal/audit/application"
	"github.com/zhanghaiyang/adflow/internal/audit/domain"
	identityhttp "github.com/zhanghaiyang/adflow/internal/identity/adapter/http"
	identity "github.com/zhanghaiyang/adflow/internal/identity/domain"
	httptransport "github.com/zhanghaiyang/adflow/internal/transport/http"
)

func TestDeleteRoutesAreAudited(t *testing.T) {
	for route, want := range map[string]string{"/v1/campaigns/:id": "DELETE_CAMPAIGN", "/v1/campaigns/:id/creatives/:creativeId": "DELETE_CREATIVE", "/v1/profiles/:userId": "DELETE_PROFILE"} {
		action, _, ok := auditedAction(http.MethodDelete, route)
		if !ok || action != want {
			t.Fatalf("route %s action %s", route, action)
		}
	}
}

func TestCreativeEnableIsAudited(t *testing.T) {
	action, resource, ok := auditedAction(http.MethodPost, "/v1/campaigns/:id/creatives/:creativeId/enable")
	if !ok || action != "ENABLE_CREATIVE" || resource != "creative" {
		t.Fatalf("%s %s %v", action, resource, ok)
	}
}

func TestMiddlewareRecordsSuccessfulAndFailedMutations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := memory.NewStore()
	service := application.NewService(store)
	router := gin.New()
	router.Use(httptransport.RequestID(), func(c *gin.Context) {
		identityhttp.SetPrincipal(c, identity.Principal{UserID: "operator-1", Username: "operator", Role: identity.RoleOperator})
		c.Next()
	}, Middleware(service))
	router.POST("/v1/campaigns/:id/pause", func(c *gin.Context) {
		httptransport.SetAuditMetadata(c, map[string]string{"provider": "openai-compatible"})
		c.Status(http.StatusOK)
	})
	router.POST("/v1/campaigns/:id/resume", func(c *gin.Context) { c.Status(http.StatusUnprocessableEntity) })

	for _, path := range []string{"/v1/campaigns/campaign-1/pause", "/v1/campaigns/campaign-2/resume"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, nil))
	}
	entries, err := store.List(t.Context(), domain.Filter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries=%d", len(entries))
	}
	if entries[0].Action != "RESUME_CAMPAIGN" || entries[0].Outcome != domain.OutcomeFailed || entries[0].ResourceID != "campaign-2" {
		t.Fatalf("unexpected failed entry: %+v", entries[0])
	}
	if entries[1].Action != "PAUSE_CAMPAIGN" || entries[1].Outcome != domain.OutcomeSucceeded {
		t.Fatalf("unexpected successful entry: %+v", entries[1])
	}
	if entries[1].Metadata["provider"] != "openai-compatible" {
		t.Fatalf("missing enriched audit metadata: %+v", entries[1].Metadata)
	}
}
