package httptransport

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/zhanghaiyang/adflow/internal/health"
)

type HealthHandler struct {
	service *health.Service
}

func NewHealthHandler(service *health.Service) *HealthHandler {
	return &HealthHandler{service: service}
}

func (h *HealthHandler) Live(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "alive"})
}

func (h *HealthHandler) Ready(c *gin.Context) {
	status := h.service.Check(c.Request.Context())
	code := http.StatusOK
	if !status.Ready {
		code = http.StatusServiceUnavailable
	}
	c.JSON(code, status)
}
