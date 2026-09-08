package application

import (
	"net/http"

	"github.com/umi3730/adflow/internal/identity/domain"
)

func requiredRole(method, route string) (domain.Role, bool) {
	if route == "/v1/simulations/decisions" {
		return domain.RoleAdmin, method == http.MethodPost
	}
	if method == http.MethodDelete && (route == "/v1/campaigns/:id" || route == "/v1/campaigns/:id/creatives/:creativeId" || route == "/v1/profiles/:userId") {
		return domain.RoleAdmin, true
	}
	if route == "/v1/audit-logs" {
		return domain.RoleAdmin, method == http.MethodGet
	}
	if route == "/v1/campaigns/:id/publish" {
		return domain.RoleAdmin, method == http.MethodPost
	}
	if route == "/v1/operations/dead-letters/:eventId/replay" {
		return domain.RoleAdmin, method == http.MethodPost
	}
	if route == "/v1/auth/me" {
		return domain.RoleViewer, method == http.MethodGet
	}
	switch method {
	case http.MethodGet:
		return domain.RoleViewer, true
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return domain.RoleOperator, true
	default:
		return "", false
	}
}
