package httpadapter

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/umi3730/adflow/internal/campaign/domain"
	transport "github.com/umi3730/adflow/internal/transport/http"
)

func TestCampaignErrorsSeparateStorageFailuresFromRuleViolations(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"storage failure", errors.New("mysql private-host:3306: internal SQL failed"), 500, "campaign_failed"},
		{"rule detail", fmt.Errorf("%w: unsupported field", domain.ErrInvalidTargeting), 422, "campaign_rule_violation"},
		{"auction", domain.ErrInvalidAuction, 422, "campaign_rule_violation"},
		{"conflict", domain.ErrConcurrentMutation, 409, "concurrent_mutation"},
		{"missing", domain.ErrCampaignNotFound, 404, "campaign_not_found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.Use(transport.RequestID())
			router.GET("/", func(c *gin.Context) { handleError(c, tc.err) })
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Header.Set("X-Request-ID", "error-review")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			body := response.Body.String()
			if response.Code != tc.status || !strings.Contains(body, tc.code) || !strings.Contains(body, "error-review") {
				t.Fatalf("status=%d body=%s", response.Code, body)
			}
			if strings.Contains(body, "private-host") || strings.Contains(body, "internal SQL") {
				t.Fatalf("internal error leaked: %s", body)
			}
			if tc.name == "rule detail" && !strings.Contains(body, "unsupported field") {
				t.Fatalf("validation detail was lost: %s", body)
			}
		})
	}
}
