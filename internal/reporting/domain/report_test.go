package domain

import (
	"testing"
	"time"
)

func TestReportBucketsAndWeightedRates(t *testing.T) {
	from := time.Date(2026, 9, 8, 16, 0, 0, 0, time.UTC)
	f := Filter{From: from, To: from.Add(48 * time.Hour), Granularity: "day"}
	if err := f.Validate(); err != nil {
		t.Fatal(err)
	}
	r := Build(f, []Row{{Bucket: from, Counts: Counts{Impressions: 100, Clicks: 10, Conversions: 2, SpendFen: 501}}, {Bucket: from.Add(24 * time.Hour), Counts: Counts{Impressions: 1, Clicks: 1, SpendFen: 5}}}, from)
	if len(r.Series) != 2 || r.Summary.Impressions != 101 || r.Summary.SpendFen != 506 || r.Summary.CTR != float64(11)/101 || r.Summary.CVR != float64(2)/11 {
		t.Fatalf("bad report: %+v", r)
	}
	if BucketStart(from.Add(-time.Second), "day").Hour() != 16 {
		t.Fatal("day bucket must use UTC+8")
	}
	empty := Build(f, nil, from)
	if len(empty.Series) != 2 || empty.Summary.CTR != 0 || empty.Summary.CVR != 0 {
		t.Fatal(empty)
	}
}

func TestFilterRejectsInvalidRanges(t *testing.T) {
	now := time.Now()
	for _, f := range []Filter{{From: now, To: now}, {From: now, To: now.Add(-time.Hour)}, {From: now, To: now.Add(32 * 24 * time.Hour), Granularity: "day"}, {From: now, To: now.Add(time.Hour), Granularity: "week"}} {
		if f.Validate() == nil {
			t.Fatalf("accepted %+v", f)
		}
	}
}
