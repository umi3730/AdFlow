package domain

import "testing"

func TestDiagnosisRequiresExistingEvidence(t *testing.T) {
	c := DiagnosisContext{Evidence: []Evidence{{ID: "report"}}}
	d := Diagnosis{Summary: "结果", Recommendations: []Recommendation{{Title: "检查", Action: "检查素材", EvidenceIDs: []string{"invented"}}}}
	if ValidateDiagnosis(d, c) == nil {
		t.Fatal("accepted invented evidence")
	}
	d.Recommendations[0].EvidenceIDs = []string{"report"}
	if err := ValidateDiagnosis(d, c); err != nil {
		t.Fatal(err)
	}
}
