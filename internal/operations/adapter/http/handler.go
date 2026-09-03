package httpadapter

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zhanghaiyang/adflow/internal/event/domain"
	"github.com/zhanghaiyang/adflow/internal/operations/application"
	httptransport "github.com/zhanghaiyang/adflow/internal/transport/http"
)

type Handler struct{ service *application.Service }

func NewHandler(service *application.Service) *Handler { return &Handler{service: service} }

func (h *Handler) RegisterRoutes(group *gin.RouterGroup) {
	group.GET("/operations/outbox", h.outbox)
	group.GET("/operations/kafka-lag", h.kafkaLag)
	group.POST("/operations/dead-letters/:eventId/replay", h.replay)
}

func (h *Handler) outbox(c *gin.Context) {
	status := strings.ToUpper(strings.TrimSpace(c.Query("status")))
	if status != "" && status != "PENDING" && status != "PROCESSING" && status != "PUBLISHED" && status != "DEAD_LETTERED" {
		httptransport.RespondError(c, http.StatusBadRequest, "invalid_outbox_status", "outbox status is invalid", nil)
		return
	}
	limit, err := queryInt(c, "limit", 50, 1, 100)
	if err != nil {
		return
	}
	offset, err := queryInt(c, "offset", 0, 0, 1000000)
	if err != nil {
		return
	}
	items, stats, err := h.service.Outbox(c.Request.Context(), domain.OutboxFilter{Status: status, Limit: limit, Offset: offset})
	if err != nil {
		httptransport.RespondError(c, http.StatusInternalServerError, "outbox_query_failed", "outbox state could not be read", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "stats": stats, "limit": limit, "offset": offset})
}

func (h *Handler) kafkaLag(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"items": h.service.KafkaLag()})
}

func (h *Handler) replay(c *gin.Context) {
	eventID := strings.TrimSpace(c.Param("eventId"))
	if eventID == "" {
		httptransport.RespondError(c, http.StatusBadRequest, "invalid_event_id", "event id is required", nil)
		return
	}
	if err := h.service.ReplayDeadLetter(c.Request.Context(), eventID); err != nil {
		switch {
		case errors.Is(err, domain.ErrOutboxEntryNotFound):
			httptransport.RespondError(c, http.StatusNotFound, "outbox_entry_not_found", err.Error(), nil)
		case errors.Is(err, domain.ErrOutboxNotDeadLetter):
			httptransport.RespondError(c, http.StatusConflict, "outbox_not_dead_lettered", err.Error(), nil)
		default:
			httptransport.RespondError(c, http.StatusInternalServerError, "dead_letter_replay_failed", "dead letter could not be replayed", nil)
		}
		return
	}
	httptransport.SetAuditMetadata(c, map[string]string{"event_id": eventID, "result": "queued"})
	c.JSON(http.StatusAccepted, gin.H{"eventId": eventID, "status": "PENDING"})
}

func queryInt(c *gin.Context, key string, fallback, minimum, maximum int) (int, error) {
	raw := c.Query(key)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < minimum || value > maximum {
		httptransport.RespondError(c, http.StatusBadRequest, "invalid_pagination", key+" is invalid", nil)
		return 0, errors.New("invalid pagination")
	}
	return value, nil
}
