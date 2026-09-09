package httpadapter

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/umi3730/adflow/internal/reporting/application"
	"github.com/umi3730/adflow/internal/reporting/domain"
	"net/http/httptest"
	"strings"
	"testing"
)

type reader struct{}

func (reader) ReadDeliveryRows(_ context.Context, f domain.Filter) ([]domain.Row, error) {
	return []domain.Row{{Bucket: f.From, Counts: domain.Counts{Impressions: 2, Clicks: 1, SpendFen: 15}}}, nil
}
func TestReportJSONCSVAndValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NewHandler(application.NewService(reader{})).RegisterRoutes(r.Group("/v1"))
	query := "?from=2026-09-08T16:00:00Z&to=2026-09-09T16:00:00Z&granularity=hour"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/v1/reports/delivery"+query, nil))
	var report domain.Report
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || len(report.Series) != 24 || report.Summary.SpendFen != 15 || report.Series[0].Bucket.In(domain.Beijing).Hour() != 0 {
		t.Fatal(w.Body.String())
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/v1/reports/delivery/export"+query, nil))
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(w.Body.String(), "\ufeff"))).ReadAll()
	if err != nil || w.Code != 200 || len(rows) != 25 || rows[1][4] != "0.1500" || rows[1][0] != "2026-09-09 00:00:00" {
		t.Fatal(rows, err)
	}
	for _, q := range []string{"", "?from=bad&to=bad", "?from=2026-01-01T00:00:00Z&to=2026-09-09T00:00:00Z"} {
		w = httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/v1/reports/delivery"+q, nil))
		if w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
}
