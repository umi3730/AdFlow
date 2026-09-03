package httpadapter

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zhanghaiyang/adflow/internal/identity/domain"
	httptransport "github.com/zhanghaiyang/adflow/internal/transport/http"
)

const principalKey = "identity_principal"

type AccessService interface {
	Authenticate(context.Context, string) (domain.Principal, error)
	Authorize(domain.Principal, string, string) bool
}

func Authentication(enabled bool, service AccessService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !enabled {
			SetPrincipal(c, domain.Principal{UserID: "local-dev", Username: "local-dev", Role: domain.RoleAdmin})
			c.Next()
			return
		}
		header := strings.TrimSpace(c.GetHeader("Authorization"))
		parts := strings.Fields(header)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			unauthorized(c)
			return
		}
		principal, err := service.Authenticate(c.Request.Context(), parts[1])
		if err != nil {
			unauthorized(c)
			return
		}
		SetPrincipal(c, principal)
		c.Next()
	}
}

func Authorization(service AccessService) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, ok := PrincipalFrom(c)
		if !ok {
			unauthorized(c)
			return
		}
		if !service.Authorize(principal, c.Request.Method, c.FullPath()) {
			c.Abort()
			httptransport.RespondError(c, http.StatusForbidden, "forbidden", "the current role cannot perform this action", nil)
			return
		}
		c.Next()
	}
}

func SetPrincipal(c *gin.Context, principal domain.Principal) {
	c.Set(principalKey, principal)
}

func PrincipalFrom(c *gin.Context) (domain.Principal, bool) {
	value, exists := c.Get(principalKey)
	if !exists {
		return domain.Principal{}, false
	}
	principal, ok := value.(domain.Principal)
	return principal, ok
}

func unauthorized(c *gin.Context) {
	c.Header("WWW-Authenticate", `Bearer realm="adflow"`)
	c.Abort()
	httptransport.RespondError(c, http.StatusUnauthorized, "unauthorized", "a valid bearer token is required", nil)
}
