package httpadapter

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zhanghaiyang/adflow/internal/decision/application"
	"github.com/zhanghaiyang/adflow/internal/decision/domain"
	httptransport "github.com/zhanghaiyang/adflow/internal/transport/http"
)

type Handler struct {
	service  application.DecisionEngine
	profiles domain.ProfileStore
	observer interface {
		ObserveDecision(bool, string, time.Duration)
	}
}

func NewHandler(service application.DecisionEngine, profiles domain.ProfileStore, observer interface {
	ObserveDecision(bool, string, time.Duration)
}) *Handler {
	return &Handler{service: service, profiles: profiles, observer: observer}
}

type decisionRequest struct {
	RequestID string `json:"requestId" binding:"required,max=128"`
	UserID    string `json:"userId" binding:"required,max=128"`
	SlotID    string `json:"slotId" binding:"required,max=64"`
}

type profileRequest struct {
	Tags   []string          `json:"tags" binding:"max=100,dive,min=1,max=64"`
	Fields map[string]string `json:"fields" binding:"max=100"`
}

func (h *Handler) RegisterRoutes(group *gin.RouterGroup) {
	group.POST("/decisions", h.decide)
	group.PUT("/profiles/:userId", h.putProfile)
}

func (h *Handler) decide(c *gin.Context) {
	started := time.Now()
	var request decisionRequest
	if !httptransport.BindJSON(c, &request) {
		return
	}
	result, err := h.service.Decide(c.Request.Context(), domain.Request{
		RequestID: request.RequestID, UserID: request.UserID, SlotID: request.SlotID,
	})
	if err != nil {
		handleError(c, err)
		return
	}
	if h.observer != nil {
		h.observer.ObserveDecision(result.Matched, string(result.Reason), time.Since(started))
	}
	c.JSON(http.StatusOK, gin.H{
		"requestId": result.RequestID, "matched": result.Matched, "campaignId": result.CampaignID,
		"creativeId": result.CreativeID, "reservationToken": result.ReservationToken,
		"expiresAt": result.ExpiresAt, "reason": result.Reason,
	})
}

func (h *Handler) putProfile(c *gin.Context) {
	var request profileRequest
	if !httptransport.BindJSON(c, &request) {
		return
	}
	userID := c.Param("userId")
	if userID == "" {
		httptransport.RespondError(c, http.StatusBadRequest, "invalid_user_id", "user id is required", nil)
		return
	}
	if err := h.profiles.PutProfile(c.Request.Context(), domain.NewProfile(userID, request.Tags, request.Fields)); err != nil {
		handleError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrRateLimited):
		c.Header("Retry-After", "1")
		httptransport.RespondError(c, http.StatusTooManyRequests, "decision_rate_limited", "decision request rate exceeded", nil)
	case errors.Is(err, domain.ErrOverloaded):
		c.Header("Retry-After", "1")
		httptransport.RespondError(c, http.StatusServiceUnavailable, "decision_overloaded", "decision service is at capacity", nil)
	case errors.Is(err, domain.ErrAdmissionUnavailable):
		httptransport.RespondError(c, http.StatusServiceUnavailable, "decision_admission_unavailable", "decision admission control is unavailable", nil)
	case errors.Is(err, domain.ErrDecisionTimeout):
		httptransport.RespondError(c, http.StatusGatewayTimeout, "decision_timeout", "decision request exceeded its deadline", nil)
	case errors.Is(err, context.Canceled):
		httptransport.RespondError(c, http.StatusRequestTimeout, "decision_canceled", "decision request was canceled", nil)
	case errors.Is(err, domain.ErrInvalidRequest):
		httptransport.RespondError(c, http.StatusUnprocessableEntity, "invalid_decision_request", err.Error(), nil)
	default:
		httptransport.RespondError(c, http.StatusInternalServerError, "decision_failed", "decision could not be completed", nil)
	}
}
