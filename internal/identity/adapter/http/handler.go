package httpadapter

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/zhanghaiyang/adflow/internal/identity/application"
	"github.com/zhanghaiyang/adflow/internal/identity/domain"
	httptransport "github.com/zhanghaiyang/adflow/internal/transport/http"
)

type Handler struct {
	service *application.Service
	enabled bool
}

func NewHandler(service *application.Service, enabled bool) *Handler {
	return &Handler{service: service, enabled: enabled}
}

type loginRequest struct {
	Username string `json:"username" binding:"required,min=2,max=64"`
	Password string `json:"password" binding:"required,min=8,max=256"`
}

func (h *Handler) RegisterRoutes(group *gin.RouterGroup) {
	group.POST("/auth/login", h.login)
	group.Use(Authentication(h.enabled, h.service), Authorization(h.service))
	group.GET("/auth/me", h.me)
}

func (h *Handler) login(c *gin.Context) {
	var request loginRequest
	if !httptransport.BindJSON(c, &request) {
		return
	}
	token, err := h.service.Login(c.Request.Context(), request.Username, request.Password)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidCredentials) || errors.Is(err, domain.ErrInactiveUser) {
			httptransport.RespondError(c, http.StatusUnauthorized, "invalid_credentials", "invalid username or password", nil)
			return
		}
		httptransport.RespondError(c, http.StatusInternalServerError, "login_failed", "login could not be completed", nil)
		return
	}
	c.JSON(http.StatusOK, token)
}

func (h *Handler) me(c *gin.Context) {
	principal, _ := PrincipalFrom(c)
	c.JSON(http.StatusOK, gin.H{"authEnabled": h.enabled, "principal": principal})
}
