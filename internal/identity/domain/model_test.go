package domain

import "testing"

func TestRoleHierarchy(t *testing.T) {
	if !RoleAdmin.Allows(RoleOperator) || !RoleOperator.Allows(RoleViewer) {
		t.Fatal("higher roles should inherit lower-role permissions")
	}
	if RoleViewer.Allows(RoleOperator) || RoleOperator.Allows(RoleAdmin) {
		t.Fatal("lower roles should not inherit higher-role permissions")
	}
	if _, ok := ParseRole("owner"); ok {
		t.Fatal("unknown role should be rejected")
	}
}
