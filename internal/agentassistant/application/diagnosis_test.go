package application_test

import (
	"context"
	"errors"
	mock "github.com/umi3730/adflow/internal/agentassistant/adapter/mock"
	app "github.com/umi3730/adflow/internal/agentassistant/application"
	ad "github.com/umi3730/adflow/internal/agentassistant/domain"
	cm "github.com/umi3730/adflow/internal/campaign/adapter/memory"
	cd "github.com/umi3730/adflow/internal/campaign/domain"
	dd "github.com/umi3730/adflow/internal/decision/domain"
	ops "github.com/umi3730/adflow/internal/operations/application"
	rd "github.com/umi3730/adflow/internal/reporting/domain"
	"testing"
	"time"
)

type reportReader struct{}

func (reportReader) Read(_ context.Context, f rd.Filter) (rd.Report, error) {
	if err := f.Validate(); err != nil {
		return rd.Report{}, err
	}
	return rd.Build(f, nil, time.Now()), nil
}

type traceReader struct{}

func (traceReader) Read(context.Context, string) (ops.RequestTrace, error) {
	return ops.RequestTrace{Decision: &ops.TraceDecision{Reason: dd.ReasonFrequencyCapped}}, nil
}

func TestDiagnosisUsesReportConfigAndSavedReason(t *testing.T) {
	repo := cm.NewRepository()
	name, _ := cd.NewName("诊断计划")
	slot, _ := cd.NewSlotID("banner")
	period, _ := cd.NewDeliveryPeriod(time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	repo.Create(t.Context(), cd.NewCampaign("c", name, slot, period))
	service := app.NewDiagnosisService(reportReader{}, repo, repo, traceReader{}, mock.NewProvider())
	f := rd.Filter{From: time.Now().Add(-time.Hour), To: time.Now(), Granularity: "hour", CampaignID: "c"}
	d, err := service.Diagnose(t.Context(), app.DiagnosisRequest{Filter: f, RequestID: "r"})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, e := range d.Evidence {
		ids[e.ID] = true
	}
	for _, id := range []string{"report", "no_impressions", "inactive", "no_creative", "request"} {
		if !ids[id] {
			t.Fatalf("missing %s", id)
		}
	}
	if len(d.Recommendations) == 0 {
		t.Fatal("no recommendations")
	}
	current, _ := repo.FindByID(t.Context(), "c")
	if current.Status() != cd.StatusDraft {
		t.Fatal("diagnosis mutated campaign")
	}
}

type failedProvider struct{}

func (failedProvider) Generate(context.Context, string) (ad.Draft, error) {
	return ad.Draft{}, errors.New("offline")
}
func (failedProvider) Diagnose(context.Context, ad.DiagnosisContext) (ad.Diagnosis, error) {
	return ad.Diagnosis{}, errors.New("offline")
}
func TestDiagnosisFallbackAndCircuit(t *testing.T) {
	p, err := app.NewResilientProvider(failedProvider{}, mock.NewProvider(), app.ResilienceConfig{PrimaryProvider: "remote", PrimaryModel: "test", FailureThreshold: 1, OpenDuration: time.Minute, FallbackEnabled: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	c := ad.DiagnosisContext{Evidence: []ad.Evidence{{ID: "report", Title: "检查", Suggestion: "检查数据"}}}
	for i := 0; i < 2; i++ {
		d, err := p.Diagnose(t.Context(), c)
		if err != nil || !d.Fallback || d.Provider != "mock" {
			t.Fatal(d, err)
		}
	}
}
