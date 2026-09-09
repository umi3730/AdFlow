package application

import (
	"github.com/umi3730/adflow/internal/identity/domain"
	"testing"
)

func TestReportAndDiagnosisRoles(t *testing.T) {
	s := NewService(nil, nil, nil)
	viewer := domain.Principal{Role: domain.RoleViewer}
	operator := domain.Principal{Role: domain.RoleOperator}
	for _, path := range []string{"/v1/reports/delivery", "/v1/reports/delivery/export"} {
		if !s.Authorize(viewer, "GET", path) {
			t.Fatal(path)
		}
	}
	if s.Authorize(viewer, "POST", "/v1/agent/delivery-diagnoses") || !s.Authorize(operator, "POST", "/v1/agent/delivery-diagnoses") {
		t.Fatal("diagnosis must require operator")
	}
}
