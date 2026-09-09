package memory

import (
	dd "github.com/umi3730/adflow/internal/decision/domain"
	ed "github.com/umi3730/adflow/internal/event/domain"
	rd "github.com/umi3730/adflow/internal/reporting/domain"
	"testing"
	"time"
)

func TestDeliveryReportCountsOnlyProcessedAndUsesSettlementPrice(t *testing.T) {
	s := NewStore()
	at := time.Date(2026, 9, 9, 1, 0, 0, 0, time.UTC)
	e := ed.Event{EventID: "e", RequestID: "r", CampaignID: "c", CreativeID: "k", Type: ed.Impression, OccurredAt: at}
	d := dd.Result{RequestID: "r", CampaignID: "c", Pricing: dd.Pricing{PriceFen: 7}}
	if _, err := s.PrepareImpression(t.Context(), e, d, at); err != nil {
		t.Fatal(err)
	}
	f := rd.Filter{From: at, To: at.Add(time.Hour), Granularity: "hour", CampaignID: "c"}
	rows, err := s.ReadDeliveryRows(t.Context(), f)
	if err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
	if _, err = s.CommitImpression(t.Context(), "e"); err != nil {
		t.Fatal(err)
	}
	s.CommitImpression(t.Context(), "e")
	s.Record(t.Context(), ed.Event{EventID: "click", RequestID: "r", CampaignID: "c", Type: ed.Click, OccurredAt: at.Add(time.Minute)})
	s.Record(t.Context(), ed.Event{EventID: "other", RequestID: "r2", CampaignID: "other", Type: ed.Conversion, ValueFen: 100, OccurredAt: at})
	s.Record(t.Context(), ed.Event{EventID: "outside", RequestID: "r", CampaignID: "c", Type: ed.Click, OccurredAt: f.To})
	rows, err = s.ReadDeliveryRows(t.Context(), f)
	if err != nil || len(rows) != 1 || rows[0].SpendFen != 7 || rows[0].Impressions != 1 || rows[0].Clicks != 1 || rows[0].ValueFen != 0 {
		t.Fatal(rows, err)
	}
}
