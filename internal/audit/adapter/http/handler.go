package httpadapter

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/umi3730/adflow/internal/audit/application"
	"github.com/umi3730/adflow/internal/audit/domain"
	httptransport "github.com/umi3730/adflow/internal/transport/http"
)

type Handler struct{ service *application.Service }

func NewHandler(service *application.Service) *Handler { return &Handler{service: service} }

func (h *Handler) RegisterRoutes(group *gin.RouterGroup) {
	group.Use(Middleware(h.service))
	group.GET("/audit-logs", h.list)
}

func (h *Handler) list(c *gin.Context) {
	limit, ok := queryInt(c, "limit", 50, 100)
	if !ok || limit == 0 {
		if ok {
			httptransport.RespondError(c, http.StatusBadRequest, "invalid_query", "limit must be greater than zero", nil)
		}
		return
	}
	offset, ok := queryInt(c, "offset", 0, 1000000)
	if !ok {
		return
	}
	entries, err := h.service.List(c.Request.Context(), domain.Filter{Limit: limit, Offset: offset})
	if err != nil {
		httptransport.RespondError(c, http.StatusInternalServerError, "audit_log_failed", "audit logs could not be read", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": entries, "limit": limit, "offset": offset})
}

func queryInt(c *gin.Context, name string, fallback, maximum int) (int, bool) {
	raw := c.Query(name)
	if raw == "" {
		return fallback, true
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 || value > maximum {
		httptransport.RespondError(c, http.StatusBadRequest, "invalid_query", name+" is outside the allowed range", nil)
		return 0, false
	}
	return value, true
}
