package httpadapter

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/umi3730/adflow/internal/decision/adapter/requestprofile"
	"github.com/umi3730/adflow/internal/decision/application"
	"github.com/umi3730/adflow/internal/decision/domain"
	"github.com/umi3730/adflow/internal/profile/schema"
	httptransport "github.com/umi3730/adflow/internal/transport/http"
	"net"
	"time"
)

type SimulationHandler struct {
	*Handler
	local bool
}

func NewSimulationHandler(service application.DecisionEngine, observer interface {
	ObserveDecision(bool, string, time.Duration)
}, environment string) *SimulationHandler {
	return &SimulationHandler{Handler: NewHandler(service, nil, observer), local: environment == "local" || environment == "test"}
}
func (h *SimulationHandler) RegisterRoutes(group *gin.RouterGroup) {
	if h.local {
		group.POST("/simulations/decisions", h.simulate)
	}
}

func (h *SimulationHandler) simulate(c *gin.Context) {
	host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		host = c.Request.RemoteAddr
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		httptransport.RespondError(c, 403, "local_simulation_only", "临时用户模拟仅允许本机访问", nil)
		return
	}
	var input struct {
		RunID     string `json:"runId" binding:"required,max=80"`
		RequestID string `json:"requestId" binding:"required,max=128"`
		SlotID    string `json:"slotId" binding:"required,max=64"`
		Profile   struct {
			UserID string            `json:"userId" binding:"required,max=128"`
			Tags   []string          `json:"tags" binding:"max=100,dive,min=1,max=64"`
			Fields map[string]string `json:"fields" binding:"max=100"`
		} `json:"profile" binding:"required"`
	}
	if !httptransport.BindJSON(c, &input) {
		return
	}
	if err := domain.ValidateSimulationIdentity(input.RunID, input.Profile.UserID); err != nil {
		message := "临时用户 ID 必须为 user-0001 ～ user-0100"
		if errors.Is(err, domain.ErrSimulationUserOutOfRange) {
			message = "临时用户序号超出范围"
		}
		httptransport.RespondError(c, 422, "invalid_simulation_identity", message, nil)
		return
	}
	if err := schema.ValidateFields(input.Profile.Fields); err != nil {
		httptransport.RespondError(c, 422, "invalid_profile_fields", err.Error(), nil)
		return
	}
	userID, requestID := domain.SimulationIdentity(input.RunID, input.Profile.UserID, input.RequestID)
	profile := domain.NewProfile(userID, input.Profile.Tags, input.Profile.Fields)
	ctx := requestprofile.WithProfile(c.Request.Context(), profile)
	started := time.Now()
	result, err := h.service.Decide(ctx, domain.Request{RequestID: requestID, UserID: userID, SlotID: input.SlotID, ProfileDigest: domain.ProfileDigest(profile)})
	if err != nil {
		handleError(c, err)
		return
	}
	if h.observer != nil {
		h.observer.ObserveDecision(result.Matched, string(result.Reason), time.Since(started))
	}
	httptransport.SetAuditMetadata(c, map[string]string{"simulation_run": input.RunID, "temporary_user_id": input.Profile.UserID})
	writeDecision(c, result)
}
