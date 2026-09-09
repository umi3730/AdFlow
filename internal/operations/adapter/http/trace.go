package httpadapter

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	decisiondomain "github.com/umi3730/adflow/internal/decision/domain"
	"github.com/umi3730/adflow/internal/operations/application"
	httptransport "github.com/umi3730/adflow/internal/transport/http"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

func (h *Handler) requestTrace(c *gin.Context) {
	if h.trace == nil {
		httptransport.RespondError(c, 503, "request_trace_unavailable", "请求详情暂不可用", nil)
		return
	}
	rawID := c.Query("requestId")
	id := strings.TrimSpace(rawID)
	run, user := c.Query("simulationRunId"), c.Query("simulationUserId")
	if rawID == "" || (run == "" && user == "" && id == "") || !utf8.ValidString(rawID) || utf8.RuneCountInString(rawID) > 128 {
		httptransport.RespondError(c, 400, "invalid_request_id", "请填写不超过 128 个字符的请求 ID", nil)
		return
	}
	if run != "" || user != "" {
		if err := decisiondomain.ValidateSimulationIdentity(run, user); err != nil {
			message := "临时用户查询参数不完整或无效"
			if errors.Is(err, decisiondomain.ErrSimulationUserOutOfRange) {
				message = "临时用户序号超出范围"
			}
			httptransport.RespondError(c, 400, "invalid_simulation_identity", message, nil)
			return
		}
		_, id = decisiondomain.SimulationIdentity(run, user, rawID)
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	trace, err := h.trace.Read(ctx, id)
	if err != nil {
		switch {
		case errors.Is(err, application.ErrTraceNotFound):
			httptransport.RespondError(c, 404, "request_trace_not_found", "尚无已保存记录；请求可能未完成，或当前进程已不再保留它。可以稍后刷新。", nil)
		case errors.Is(err, application.ErrInvalidTraceID):
			httptransport.RespondError(c, 400, "invalid_request_id", err.Error(), nil)
		case errors.Is(err, application.ErrTraceUnavailable):
			httptransport.RespondError(c, 503, "request_trace_unavailable", "请求详情暂不可用", nil)
		case errors.Is(err, context.DeadlineExceeded):
			httptransport.RespondError(c, 504, "request_trace_timeout", "查询超时，请稍后刷新", nil)
		default:
			httptransport.RespondError(c, 500, "request_trace_failed", "无法读取请求详情，请稍后重试", nil)
		}
		return
	}
	c.JSON(http.StatusOK, trace)
}
