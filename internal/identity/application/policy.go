package application

import (
	"net/http"

	"github.com/zhanghaiyang/adflow/internal/identity/domain"
)

func requiredRole(method, route string) (domain.Role, bool) {
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
