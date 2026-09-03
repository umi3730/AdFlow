package httpadapter

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zhanghaiyang/adflow/internal/event/domain"
	httptransport "github.com/zhanghaiyang/adflow/internal/transport/http"
)

type Handler struct {
	service interface {
		Record(context.Context, domain.Event) (bool, error)
		Metrics(context.Context, string) (domain.Metrics, error)
	}
	observer interface{ ObserveEvent(string, bool) }
	async    bool
}

func NewHandler(service interface {
	Record(context.Context, domain.Event) (bool, error)
	Metrics(context.Context, string) (domain.Metrics, error)
}, observer interface{ ObserveEvent(string, bool) }, async bool) *Handler {
	return &Handler{service: service, observer: observer, async: async}
}

type eventRequest struct {
	EventID    string      `json:"eventId" binding:"required,max=128"`
	RequestID  string      `json:"requestId" binding:"required,max=128"`
	CampaignID string      `json:"campaignId" binding:"required,max=128"`
	CreativeID string      `json:"creativeId" binding:"required,max=128"`
	Type       domain.Type `json:"type" binding:"required,oneof=impression click conversion"`
	ValueFen   int64       `json:"valueFen" binding:"gte=0"`
	OccurredAt *time.Time  `json:"occurredAt"`
}

func (h *Handler) RegisterRoutes(group *gin.RouterGroup) {
	group.POST("/events", h.record)
	group.GET("/campaigns/:id/metrics", h.metrics)
}

func (h *Handler) record(c *gin.Context) {
	var request eventRequest
	if !httptransport.BindJSON(c, &request) {
		return
	}
	event := domain.Event{
		EventID: request.EventID, RequestID: request.RequestID, CampaignID: request.CampaignID,
		CreativeID: request.CreativeID, Type: request.Type, ValueFen: request.ValueFen,
	}
	if request.OccurredAt != nil {
		event.OccurredAt = *request.OccurredAt
	}
	created, err := h.service.Record(c.Request.Context(), event)
	if err != nil {
		handleError(c, err)
		return
	}
	if h.observer != nil {
		h.observer.ObserveEvent(string(request.Type), created)
	}
	status := http.StatusCreated
	if h.async {
		status = http.StatusAccepted
	}
	if !created {
		status = http.StatusOK
	}
	c.JSON(status, gin.H{"eventId": request.EventID, "recorded": created})
}

func (h *Handler) metrics(c *gin.Context) {
	metrics, err := h.service.Metrics(c.Request.Context(), c.Param("id"))
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, metrics)
}

func handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidEvent):
		httptransport.RespondError(c, http.StatusUnprocessableEntity, "invalid_event", err.Error(), nil)
	case errors.Is(err, domain.ErrDecisionNotFound):
		httptransport.RespondError(c, http.StatusNotFound, "decision_not_found", err.Error(), nil)
	case errors.Is(err, domain.ErrImpressionRequired):
		httptransport.RespondError(c, http.StatusConflict, "impression_required", err.Error(), nil)
	default:
		httptransport.RespondError(c, http.StatusInternalServerError, "event_failed", "event could not be recorded", nil)
	}
}
