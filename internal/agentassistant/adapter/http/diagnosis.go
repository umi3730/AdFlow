package httpadapter

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/umi3730/adflow/internal/agentassistant/application"
	ad "github.com/umi3730/adflow/internal/agentassistant/domain"
	cd "github.com/umi3730/adflow/internal/campaign/domain"
	ops "github.com/umi3730/adflow/internal/operations/application"
	rd "github.com/umi3730/adflow/internal/reporting/domain"
	transport "github.com/umi3730/adflow/internal/transport/http"
	"log/slog"
	"strconv"
)

type DiagnosisHandler struct{ service *application.DiagnosisService }

func NewDiagnosisHandler(s *application.DiagnosisService) *DiagnosisHandler {
	return &DiagnosisHandler{service: s}
}
func (h *DiagnosisHandler) RegisterRoutes(g *gin.RouterGroup) {
	g.POST("/agent/delivery-diagnoses", h.diagnose)
}
func (h *DiagnosisHandler) diagnose(c *gin.Context) {
	var input application.DiagnosisRequest
	if !transport.BindJSON(c, &input) {
		return
	}
	result, err := h.service.Diagnose(c.Request.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, ad.ErrInvalidDiagnosis), errors.Is(err, rd.ErrInvalidFilter), errors.Is(err, ops.ErrInvalidTraceID):
			transport.RespondError(c, 422, "invalid_diagnosis", err.Error(), nil)
		case errors.Is(err, cd.ErrCampaignNotFound), errors.Is(err, ops.ErrTraceNotFound):
			transport.RespondError(c, 404, "diagnosis_subject_not_found", "未找到所选计划或请求记录", nil)
		default:
			slog.Error("delivery diagnosis failed", "error", err)
			transport.RespondError(c, 503, "diagnosis_unavailable", "暂时无法完成诊断，请稍后重试", nil)
		}
		return
	}
	transport.SetAuditMetadata(c, map[string]string{"provider": result.Provider, "model": result.Model, "prompt_version": result.PromptVersion, "fallback": strconv.FormatBool(result.Fallback), "total_tokens": strconv.FormatInt(result.TotalTokens, 10)})
	c.JSON(200, result)
}
