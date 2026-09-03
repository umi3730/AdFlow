package httpadapter

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/zhanghaiyang/adflow/internal/agentassistant/application"
	"github.com/zhanghaiyang/adflow/internal/agentassistant/domain"
	httptransport "github.com/zhanghaiyang/adflow/internal/transport/http"
)

type Handler struct {
	service *application.Service
}

func NewHandler(service *application.Service) *Handler { return &Handler{service: service} }

type generateRequest struct {
	Prompt string `json:"prompt" binding:"required,min=5,max=2000"`
}

func (h *Handler) RegisterRoutes(group *gin.RouterGroup) {
	group.POST("/agent/rule-drafts", h.generate)
}

func (h *Handler) generate(c *gin.Context) {
	var request generateRequest
	if !httptransport.BindJSON(c, &request) {
		return
	}
	draft, err := h.service.Generate(c.Request.Context(), request.Prompt)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidPrompt) || errors.Is(err, domain.ErrInvalidDraft) {
			httptransport.RespondError(c, http.StatusUnprocessableEntity, "invalid_rule_draft", err.Error(), nil)
			return
		}
		httptransport.RespondError(c, http.StatusBadGateway, "rule_provider_failed", "rule provider failed", nil)
		return
	}
	c.JSON(http.StatusOK, draft)
}
