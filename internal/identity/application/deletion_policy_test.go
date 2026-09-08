package application

import (
	"github.com/zhanghaiyang/adflow/internal/identity/domain"
	"net/http"
	"testing"
)

func TestDeletionRequiresAdmin(t *testing.T) {
	for _, route := range []string{"/v1/campaigns/:id", "/v1/campaigns/:id/creatives/:creativeId", "/v1/profiles/:userId"} {
		role, ok := requiredRole(http.MethodDelete, route)
		if !ok || role != domain.RoleAdmin || domain.RoleOperator.Allows(role) || domain.RoleViewer.Allows(role) {
			t.Fatalf("delete permission for %s = %s", route, role)
		}
	}
}

func TestCreativeEnableRequiresOperator(t *testing.T) {
	role, ok := requiredRole(http.MethodPost, "/v1/campaigns/:id/creatives/:creativeId/enable")
	if !ok || role != domain.RoleOperator || domain.RoleViewer.Allows(role) || !domain.RoleOperator.Allows(role) {
		t.Fatal("incorrect enable permission")
	}
}

func TestTemporarySimulationRequiresAdmin(t *testing.T) {
	role, ok := requiredRole(http.MethodPost, "/v1/simulations/decisions")
	if !ok || role != domain.RoleAdmin || domain.RoleOperator.Allows(role) {
		t.Fatal("simulation permission too broad")
	}
}
