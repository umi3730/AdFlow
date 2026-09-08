package httpadapter

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zhanghaiyang/adflow/internal/campaign/application"
	"github.com/zhanghaiyang/adflow/internal/campaign/domain"
	httptransport "github.com/zhanghaiyang/adflow/internal/transport/http"
)

type Handler struct {
	service         *application.Service
	creativeService *application.CreativeService
}

func NewHandler(service *application.Service, creativeService *application.CreativeService) *Handler {
	return &Handler{service: service, creativeService: creativeService}
}

func (h *Handler) RegisterRoutes(group *gin.RouterGroup) {
	campaigns := group.Group("/campaigns")
	campaigns.POST("", h.create)
	campaigns.GET("", h.list)
	campaigns.GET("/:id", h.get)
	campaigns.PUT("/:id", h.update)
	campaigns.DELETE("/:id", h.deleteCampaign)
	campaigns.POST("/:id/publish", h.publish)
	campaigns.POST("/:id/pause", h.pause)
	campaigns.POST("/:id/resume", h.resume)
	campaigns.POST("/:id/creatives", h.createCreative)
	campaigns.GET("/:id/creatives", h.listCreatives)
	campaigns.POST("/:id/creatives/:creativeId/disable", h.disableCreative)
	campaigns.POST("/:id/creatives/:creativeId/enable", h.enableCreative)
	campaigns.DELETE("/:id/creatives/:creativeId", h.deleteCreative)
}

func (h *Handler) deleteCampaign(c *gin.Context) {
	if err := h.service.Delete(c.Request.Context(), c.Param("id")); err != nil {
		handleError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) deleteCreative(c *gin.Context) {
	if err := h.creativeService.Delete(c.Request.Context(), c.Param("id"), c.Param("creativeId")); err != nil {
		handleError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) create(c *gin.Context) {
	var request createRequest
	if !httptransport.BindJSON(c, &request) {
		return
	}
	campaign, err := h.service.Create(c.Request.Context(), application.CreateCommand{
		Name: request.Name, SlotID: request.SlotID, StartAt: request.StartAt, EndAt: request.EndAt,
	})
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusCreated, toResponse(campaign))
}

func (h *Handler) get(c *gin.Context) {
	campaign, err := h.service.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, toResponse(campaign))
}

func (h *Handler) update(c *gin.Context) {
	var request updateRequest
	if !httptransport.BindJSON(c, &request) {
		return
	}
	campaign, err := h.service.Update(c.Request.Context(), application.UpdateCommand{
		CampaignID: c.Param("id"), Name: request.Name, SlotID: request.SlotID,
		StartAt: request.StartAt, EndAt: request.EndAt,
	})
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, toResponse(campaign))
}

func (h *Handler) list(c *gin.Context) {
	limit, err := queryInt(c, "limit", 20)
	if err != nil {
		return
	}
	offset, err := queryInt(c, "offset", 0)
	if err != nil {
		return
	}
	var status *domain.Status
	if raw := strings.TrimSpace(c.Query("status")); raw != "" {
		parsed := domain.Status(strings.ToUpper(raw))
		if parsed != domain.StatusDraft && parsed != domain.StatusActive && parsed != domain.StatusPaused && parsed != domain.StatusEnded {
			httptransport.RespondError(c, http.StatusBadRequest, "invalid_status", "status query parameter is invalid", nil)
			return
		}
		status = &parsed
	}
	campaigns, err := h.service.List(c.Request.Context(), domain.ListFilter{Status: status, Limit: limit, Offset: offset})
	if err != nil {
		handleError(c, err)
		return
	}
	items := make([]campaignResponse, 0, len(campaigns))
	for _, campaign := range campaigns {
		items = append(items, toResponse(campaign))
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "limit": limit, "offset": offset})
}

func (h *Handler) publish(c *gin.Context) {
	var request publishRequest
	if !httptransport.BindJSON(c, &request) {
		return
	}
	campaign, err := h.service.Publish(c.Request.Context(), application.PublishCommand{
		Auction:           request.Auction,
		CampaignID:        c.Param("id"),
		All:               request.Targeting.All,
		Any:               request.Targeting.Any,
		None:              request.Targeting.None,
		DailyBudgetFen:    request.DailyBudgetFen,
		ImpressionCostFen: request.ImpressionCostFen,
		FrequencyLimit:    request.FrequencyLimit,
	})
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, toResponse(campaign))
}

func (h *Handler) pause(c *gin.Context)  { h.transition(c, h.service.Pause) }
func (h *Handler) resume(c *gin.Context) { h.transition(c, h.service.Resume) }

func (h *Handler) createCreative(c *gin.Context) {
	var request createCreativeRequest
	if !httptransport.BindJSON(c, &request) {
		return
	}
	creative, err := h.creativeService.Create(c.Request.Context(), application.CreateCreativeCommand{
		CampaignID: c.Param("id"), Title: request.Title, Description: request.Description,
		ImageURL: request.ImageURL, LandingURL: request.LandingURL,
	})
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusCreated, toCreativeResponse(creative))
}

func (h *Handler) listCreatives(c *gin.Context) {
	creatives, err := h.creativeService.List(c.Request.Context(), c.Param("id"))
	if err != nil {
		handleError(c, err)
		return
	}
	items := make([]creativeResponse, 0, len(creatives))
	for _, creative := range creatives {
		items = append(items, toCreativeResponse(creative))
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *Handler) disableCreative(c *gin.Context) {
	creative, err := h.creativeService.Disable(c.Request.Context(), c.Param("id"), c.Param("creativeId"))
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, toCreativeResponse(creative))
}

func (h *Handler) enableCreative(c *gin.Context) {
	creative, err := h.creativeService.Enable(c.Request.Context(), c.Param("id"), c.Param("creativeId"))
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, toCreativeResponse(creative))
}

func (h *Handler) transition(c *gin.Context, operation func(context.Context, string) (*domain.Campaign, error)) {
	campaign, err := operation(c.Request.Context(), c.Param("id"))
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, toResponse(campaign))
}

func handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrDeleteActive):
		httptransport.RespondError(c, http.StatusConflict, "delete_active_resource", "请先暂停计划或禁用素材，再执行删除", nil)
	case errors.Is(err, domain.ErrCampaignNotFound):
		httptransport.RespondError(c, http.StatusNotFound, "campaign_not_found", err.Error(), nil)
	case errors.Is(err, domain.ErrCreativeNotFound):
		httptransport.RespondError(c, http.StatusNotFound, "creative_not_found", err.Error(), nil)
	case errors.Is(err, domain.ErrConcurrentMutation):
		httptransport.RespondError(c, http.StatusConflict, "concurrent_mutation", err.Error(), nil)
	default:
		httptransport.RespondError(c, http.StatusUnprocessableEntity, "campaign_rule_violation", err.Error(), nil)
	}
}

func queryInt(c *gin.Context, name string, fallback int) (int, error) {
	raw := c.Query(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		httptransport.RespondError(c, http.StatusBadRequest, "invalid_query", name+" must be a non-negative integer", nil)
		return 0, errors.New("invalid query")
	}
	return value, nil
}
