package httpadapter

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/umi3730/adflow/internal/decision/application"
	"github.com/umi3730/adflow/internal/decision/domain"
	"github.com/umi3730/adflow/internal/profile/schema"
	httptransport "github.com/umi3730/adflow/internal/transport/http"
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
	group.GET("/profiles", h.listProfiles)
	group.GET("/profiles/:userId", h.getProfile)
	group.DELETE("/profiles/:userId", h.deleteProfile)
}

func (h *Handler) deleteProfile(c *gin.Context) {
	deleter, ok := h.profiles.(domain.ProfileDeleter)
	if !ok {
		httptransport.RespondError(c, 503, "profile_delete_unavailable", "画像删除暂不可用", nil)
		return
	}
	if err := deleter.DeleteProfile(c.Request.Context(), c.Param("userId")); err != nil {
		handleError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

type profileResponse struct {
	UserID string            `json:"userId"`
	Tags   []string          `json:"tags"`
	Fields map[string]string `json:"fields"`
}

func profileDTO(profile domain.Profile) profileResponse {
	tags := make([]string, 0, len(profile.Tags))
	for tag := range profile.Tags {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return profileResponse{UserID: profile.UserID, Tags: tags, Fields: profile.Fields}
}

func (h *Handler) listProfiles(c *gin.Context) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if err != nil || limit < 1 || limit > 100 {
		httptransport.RespondError(c, 400, "invalid_pagination", "limit must be between 1 and 100", nil)
		return
	}
	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil || offset < 0 || offset > 1000000 {
		httptransport.RespondError(c, 400, "invalid_pagination", "offset must be between 0 and 1000000", nil)
		return
	}
	query, tag, device := strings.TrimSpace(c.Query("q")), strings.TrimSpace(c.Query("tag")), strings.TrimSpace(c.Query("device"))
	if len(query) > 128 || len(tag) > 256 || len(device) > 128 {
		httptransport.RespondError(c, 400, "invalid_filter", "profile filter is too long", nil)
		return
	}
	catalog, ok := h.profiles.(domain.ProfileCatalog)
	if !ok {
		httptransport.RespondError(c, 503, "profile_catalog_unavailable", "profile catalog is unavailable", nil)
		return
	}
	page, err := catalog.ListProfiles(c.Request.Context(), domain.ProfileFilter{Query: query, Tag: tag, Device: device, Limit: limit, Offset: offset})
	if err != nil {
		handleError(c, err)
		return
	}
	items := make([]profileResponse, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, profileDTO(item))
	}
	c.JSON(200, gin.H{"items": items, "total": page.Total, "limit": limit, "offset": offset})
}

func (h *Handler) getProfile(c *gin.Context) {
	profile, err := h.profiles.FindProfile(c.Request.Context(), c.Param("userId"))
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(200, profileDTO(profile))
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
	writeDecision(c, result)
}

func writeDecision(c *gin.Context, result domain.Result) {
	var pricing any
	if result.Pricing.Mode != "" {
		pricing = result.Pricing
	}
	var expiresAt *time.Time
	if !result.ExpiresAt.IsZero() {
		value := result.ExpiresAt
		expiresAt = &value
	}
	c.JSON(http.StatusOK, gin.H{
		"pricing":   pricing,
		"requestId": result.RequestID, "matched": result.Matched, "campaignId": result.CampaignID,
		"creativeId": result.CreativeID, "reservationToken": result.ReservationToken,
		"expiresAt": expiresAt, "reason": result.Reason,
	})
}

func (h *Handler) putProfile(c *gin.Context) {
	var request profileRequest
	if !httptransport.BindJSON(c, &request) {
		return
	}
	if err := schema.ValidateFields(request.Fields); err != nil {
		httptransport.RespondError(c, http.StatusUnprocessableEntity, "invalid_profile_fields", err.Error(), nil)
		return
	}
	userID := strings.TrimSpace(c.Param("userId"))
	if userID == "" || utf8.RuneCountInString(userID) > 128 {
		httptransport.RespondError(c, http.StatusBadRequest, "invalid_user_id", "user id must contain 1 to 128 characters and is case-sensitive", nil)
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
	case errors.Is(err, domain.ErrBackpressure):
		c.Header("Retry-After", "1")
		httptransport.RespondError(c, http.StatusServiceUnavailable, "decision_backpressure", "后台结算或发布积压，暂时停止新决策，请稍后重试", nil)
	case errors.Is(err, domain.ErrBackpressureUnavailable):
		c.Header("Retry-After", "1")
		httptransport.RespondError(c, http.StatusServiceUnavailable, "decision_backpressure_unavailable", "暂时无法确认后台处理状态，请稍后重试", nil)
	case errors.Is(err, domain.ErrRequestConflict):
		httptransport.RespondError(c, http.StatusConflict, "request_id_conflict", "该请求 ID 已用于不同的用户、广告位或画像参数", nil)
	case errors.Is(err, domain.ErrDecisionInProgress), errors.Is(err, domain.ErrDecisionExecutionLost):
		c.Header("Retry-After", "1")
		httptransport.RespondError(c, http.StatusConflict, "decision_in_progress", "该请求正在处理或执行权已变化，请使用原参数重试", nil)
	case errors.Is(err, domain.ErrProfileNotFound):
		httptransport.RespondError(c, http.StatusNotFound, "profile_not_found", "用户画像不存在，请先保存画像", nil)
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
