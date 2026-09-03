package httptransport

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	RequestIDHeader  = "X-Request-ID"
	requestIDKey     = "request_id"
	auditMetadataKey = "audit_metadata"
)

func CORS(environment string) gin.HandlerFunc {
	allowed := map[string]struct{}{}
	if environment == "local" || environment == "test" {
		allowed["http://localhost:3000"] = struct{}{}
		allowed["http://127.0.0.1:3000"] = struct{}{}
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if _, ok := allowed[origin]; ok {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
		}
		if c.Request.Method == http.MethodOptions {
			if _, ok := allowed[origin]; !ok {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader(RequestIDHeader)
		if requestID == "" {
			requestID = newRequestID()
		}
		c.Set(requestIDKey, requestID)
		c.Header(RequestIDHeader, requestID)
		c.Next()
	}
}

func AccessLogger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()
		logger.Info("http request",
			"request_id", RequestIDFrom(c),
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"response_bytes", c.Writer.Size(),
			"duration_ms", time.Since(started).Milliseconds(),
		)
	}
}

func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("panic recovered",
					"request_id", RequestIDFrom(c),
					"error", recovered,
					"stack", string(debug.Stack()),
				)
				c.Abort()
				RespondError(c, http.StatusInternalServerError, "internal_error", "internal server error", nil)
			}
		}()
		c.Next()
	}
}

func RequestIDFrom(c *gin.Context) string {
	value, exists := c.Get(requestIDKey)
	if !exists {
		return ""
	}
	requestID, _ := value.(string)
	return requestID
}

func SetAuditMetadata(c *gin.Context, metadata map[string]string) {
	copy := make(map[string]string, len(metadata))
	for key, value := range metadata {
		copy[key] = value
	}
	c.Set(auditMetadataKey, copy)
}

func AuditMetadataFrom(c *gin.Context) map[string]string {
	value, exists := c.Get(auditMetadataKey)
	if !exists {
		return nil
	}
	metadata, _ := value.(map[string]string)
	copy := make(map[string]string, len(metadata))
	for key, item := range metadata {
		copy[key] = item
	}
	return copy
}

func newRequestID() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(data[:])
}
