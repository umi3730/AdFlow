package httpadapter

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/umi3730/adflow/internal/agentassistant/application"
	"github.com/umi3730/adflow/internal/agentassistant/domain"
	httptransport "github.com/umi3730/adflow/internal/transport/http"
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
	httptransport.SetAuditMetadata(c, map[string]string{
		"provider": draft.Provider, "model": draft.Model, "prompt_version": draft.PromptVersion,
		"fallback": strconv.FormatBool(draft.Fallback), "total_tokens": strconv.FormatInt(draft.TotalTokens, 10),
	})
	c.JSON(http.StatusOK, draft)
}
