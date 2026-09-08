package httpadapter_test

import (
	"github.com/gin-gonic/gin"
	campaignhttp "github.com/umi3730/adflow/internal/campaign/adapter/http"
	"github.com/umi3730/adflow/internal/campaign/adapter/memory"
	"github.com/umi3730/adflow/internal/campaign/application"
	decisionhttp "github.com/umi3730/adflow/internal/decision/adapter/http"
	decisionmemory "github.com/umi3730/adflow/internal/decision/adapter/memory"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPRejectsTypedFieldBypass(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := memory.NewRepository()
	service := application.NewService(repo, nil)
	now := time.Now()
	campaign, err := service.Create(t.Context(), application.CreateCommand{Name: "typed test", SlotID: "banner", StartAt: now, EndAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	group := router.Group("/v1")
	campaignhttp.NewHandler(service, application.NewCreativeService(repo, repo)).RegisterRoutes(group)
	decisionhttp.NewHandler(nil, decisionmemory.NewRuntime(), nil).RegisterRoutes(group)
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"POST", "/v1/campaigns/" + campaign.ID() + "/publish", `{"targeting":{"all":[{"field":"country","op":"gte","value":"156"}]},"dailyBudgetFen":100,"impressionCostFen":1,"frequencyLimit":1}`, 422},
		{"PUT", "/v1/profiles/user", `{"fields":{"score":"NaN"}}`, 422},
		{"PUT", "/v1/profiles/user", `{"fields":{"score":"101"}}`, 422},
		{"PUT", "/v1/profiles/user", `{"fields":{"device":"bad"}}`, 422},
		{"PUT", "/v1/profiles/user", `{"fields":{"device":"android","score":"88.5","country":"CN"}}`, 204},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != tc.status {
			t.Fatalf("%s %s got %d: %s", tc.method, tc.path, res.Code, res.Body.String())
		}
	}
	stored, _ := service.Get(t.Context(), campaign.ID())
	if stored.ActiveVersion() != nil {
		t.Fatal("invalid HTTP rule was published")
	}
}
