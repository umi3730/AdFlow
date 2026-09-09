package httpadapter

import (
	"encoding/csv"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/umi3730/adflow/internal/reporting/application"
	"github.com/umi3730/adflow/internal/reporting/domain"
	transport "github.com/umi3730/adflow/internal/transport/http"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

type Handler struct{ service *application.Service }

func NewHandler(service *application.Service) *Handler { return &Handler{service: service} }
func (h *Handler) RegisterRoutes(g *gin.RouterGroup) {
	g.GET("/reports/delivery", h.read)
	g.GET("/reports/delivery/export", h.export)
}
func ParseFilter(c *gin.Context) (domain.Filter, error) {
	from, e1 := time.Parse(time.RFC3339, c.Query("from"))
	to, e2 := time.Parse(time.RFC3339, c.Query("to"))
	f := domain.Filter{From: from, To: to, Granularity: c.DefaultQuery("granularity", "day"), CampaignID: c.Query("campaignId")}
	if e1 != nil || e2 != nil {
		return f, domain.ErrInvalidFilter
	}
	return f, f.Validate()
}
func (h *Handler) load(c *gin.Context) (domain.Report, bool) {
	f, err := ParseFilter(c)
	var r domain.Report
	if err == nil {
		r, err = h.service.Read(c.Request.Context(), f)
	}
	if err != nil {
		if errors.Is(err, domain.ErrInvalidFilter) {
			transport.RespondError(c, 400, "invalid_report_filter", err.Error(), nil)
		} else {
			slog.Error("read delivery report", "error", err)
			transport.RespondError(c, 503, "report_unavailable", "投放报表暂时不可用，请稍后重试", nil)
		}
		return r, false
	}
	return r, true
}
func (h *Handler) read(c *gin.Context) {
	if r, ok := h.load(c); ok {
		c.JSON(http.StatusOK, r)
	}
}
func (h *Handler) export(c *gin.Context) {
	r, ok := h.load(c)
	if !ok {
		return
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="adflow-delivery.csv"`)
	_, _ = c.Writer.Write([]byte{0xef, 0xbb, 0xbf})
	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"时段（北京时间）", "曝光", "点击", "转化", "消耗（元）", "转化价值（元）", "CTR（%）", "CVR（%）", "平均点击成本（元）", "平均转化成本（元）", "缺少结算凭据的曝光"})
	number := func(n float64) string { return strconv.FormatFloat(n, 'f', 4, 64) }
	for _, p := range r.Series {
		_ = w.Write([]string{p.Bucket.In(domain.Beijing).Format("2006-01-02 15:04:05"), strconv.FormatUint(p.Impressions, 10), strconv.FormatUint(p.Clicks, 10), strconv.FormatUint(p.Conversions, 10), number(float64(p.SpendFen) / 100), number(float64(p.ValueFen) / 100), number(p.CTR * 100), number(p.CVR * 100), number(p.CPCFen / 100), number(p.CPAFen / 100), strconv.FormatUint(p.UnpricedImpressions, 10)})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		slog.Error("export delivery report", "error", err)
	}
}
