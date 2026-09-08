package httpadapter

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/umi3730/adflow/internal/decision/application"
	httptransport "github.com/umi3730/adflow/internal/transport/http"
)

type ExplanationHandler struct {
	service *application.ExplanationService
}

func NewExplanationHandler(service *application.ExplanationService) *ExplanationHandler {
	return &ExplanationHandler{service: service}
}

func (h *ExplanationHandler) RegisterRoutes(group *gin.RouterGroup) {
	group.POST("/decisions/explain", h.explain)
}

func (h *ExplanationHandler) explain(c *gin.Context) {
	var request struct {
		UserID string `json:"userId" binding:"required,max=128"`
		SlotID string `json:"slotId" binding:"required,min=2,max=64"`
	}
	if !httptransport.BindJSON(c, &request) {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	result, err := h.service.Explain(ctx, request.UserID, request.SlotID)
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(200, gin.H{
		"profile": profileDTO(result.Profile), "candidates": result.Candidates, "checkedAt": result.CheckedAt,
		"scope": "current_targeting_only",
	})
}
