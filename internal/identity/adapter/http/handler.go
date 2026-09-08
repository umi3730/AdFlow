package httpadapter

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/umi3730/adflow/internal/identity/application"
	"github.com/umi3730/adflow/internal/identity/domain"
	httptransport "github.com/umi3730/adflow/internal/transport/http"
)

type Handler struct {
	service     *application.Service
	enabled     bool
	demoPrefill bool
}

func NewHandler(service *application.Service, enabled bool) *Handler {
	return &Handler{service: service, enabled: enabled}
}

func (h *Handler) SetDemoPrefill(enabled bool) { h.demoPrefill = enabled }

type loginRequest struct {
	Username string `json:"username" binding:"required,min=2,max=64"`
	Password string `json:"password" binding:"required,min=8,max=256"`
}

func (h *Handler) RegisterRoutes(group *gin.RouterGroup) {
	group.GET("/auth/options", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"registrationEnabled": h.service.RegistrationEnabled(), "registrationRole": "admin", "demoLoginPrefill": h.demoPrefill})
	})
	group.POST("/auth/register", h.register)
	group.POST("/auth/login", h.login)
	group.Use(Authentication(h.enabled, h.service), Authorization(h.service))
	group.GET("/auth/me", h.me)
}

func (h *Handler) register(c *gin.Context) {
	if !h.service.RegistrationEnabled() {
		httptransport.RespondError(c, http.StatusForbidden, "registration_disabled", "当前环境未开放注册", nil)
		return
	}
	var request loginRequest
	if !httptransport.BindJSON(c, &request) {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	token, err := h.service.Register(ctx, request.Username, request.Password)
	switch {
	case err == nil:
		c.JSON(http.StatusCreated, token)
	case errors.Is(err, domain.ErrUsernameTaken):
		httptransport.RespondError(c, http.StatusConflict, "username_taken", "用户名已存在，请换一个或直接登录", nil)
	case errors.Is(err, domain.ErrInvalidRegistration):
		httptransport.RespondError(c, http.StatusUnprocessableEntity, "invalid_registration", "用户名需为 3–32 位字母、数字、下划线或短横线，以字母或数字开头；密码至少 8 个字符且不超过 72 字节", nil)
	case errors.Is(err, domain.ErrRegistrationBusy):
		c.Header("Retry-After", "1")
		httptransport.RespondError(c, http.StatusTooManyRequests, "registration_busy", "注册请求较多，请稍后重试", nil)
	default:
		httptransport.RespondError(c, http.StatusServiceUnavailable, "registration_failed", "暂时无法完成注册，请稍后重试", nil)
	}
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
