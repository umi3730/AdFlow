package domain

import (
	"context"
	"errors"
	"strings"
	"time"
)

var ErrInvalidFilter = errors.New("请选择有效的时间范围（最多31天）和小时/天粒度")
var Beijing = time.FixedZone("Asia/Shanghai", 8*60*60)

type Filter struct {
	From        time.Time `json:"from"`
	To          time.Time `json:"to"`
	Granularity string    `json:"granularity"`
	CampaignID  string    `json:"campaignId,omitempty"`
}

func (f Filter) Validate() error {
	if f.From.IsZero() || f.To.IsZero() || !f.To.After(f.From) || f.To.Sub(f.From) > 31*24*time.Hour || (f.Granularity != "hour" && f.Granularity != "day") || len(f.CampaignID) > 64 || strings.TrimSpace(f.CampaignID) != f.CampaignID {
		return ErrInvalidFilter
	}
	return nil
}

type Counts struct {
	Impressions         uint64 `json:"impressions"`
	Clicks              uint64 `json:"clicks"`
	Conversions         uint64 `json:"conversions"`
	SpendFen            int64  `json:"spendFen"`
	ValueFen            int64  `json:"valueFen"`
	UnpricedImpressions uint64 `json:"unpricedImpressions"`
}
type Row struct {
	Bucket time.Time
	Counts
}
type Metrics struct {
	Counts
	CTR    float64 `json:"ctr"`
	CVR    float64 `json:"cvr"`
	CPCFen float64 `json:"cpcFen"`
	CPAFen float64 `json:"cpaFen"`
}
type Point struct {
	Bucket time.Time `json:"bucket"`
	Metrics
}
type Report struct {
	Filter
	Timezone    string    `json:"timezone"`
	GeneratedAt time.Time `json:"generatedAt"`
	Summary     Metrics   `json:"summary"`
	Series      []Point   `json:"series"`
}
type Reader interface {
	ReadDeliveryRows(context.Context, Filter) ([]Row, error)
}

func BucketStart(at time.Time, granularity string) time.Time {
	at = at.In(Beijing)
	if granularity == "day" {
		return time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, Beijing).UTC()
	}
	return at.Truncate(time.Hour).UTC()
}
func (c *Counts) Add(other Counts) {
	c.Impressions += other.Impressions
	c.Clicks += other.Clicks
	c.Conversions += other.Conversions
	c.SpendFen += other.SpendFen
	c.ValueFen += other.ValueFen
	c.UnpricedImpressions += other.UnpricedImpressions
}
func Rates(c Counts) Metrics {
	m := Metrics{Counts: c}
	if c.Impressions > 0 {
		m.CTR = float64(c.Clicks) / float64(c.Impressions)
	}
	if c.Clicks > 0 {
		m.CVR = float64(c.Conversions) / float64(c.Clicks)
		m.CPCFen = float64(c.SpendFen) / float64(c.Clicks)
	}
	if c.Conversions > 0 {
		m.CPAFen = float64(c.SpendFen) / float64(c.Conversions)
	}
	return m
}

// Build fills empty buckets and calculates totals before rates, never averages ratios.
func Build(f Filter, rows []Row, now time.Time) Report {
	result := Report{Filter: f, Timezone: "Asia/Shanghai", GeneratedAt: now.UTC(), Series: []Point{}}
	buckets := map[time.Time]Counts{}
	for _, row := range rows {
		key := BucketStart(row.Bucket, f.Granularity)
		c := buckets[key]
		c.Add(row.Counts)
		buckets[key] = c
	}
	step := time.Hour
	if f.Granularity == "day" {
		step = 24 * time.Hour
	}
	var totals Counts
	for at := BucketStart(f.From, f.Granularity); at.Before(f.To); at = at.Add(step) {
		c := buckets[at]
		totals.Add(c)
		result.Series = append(result.Series, Point{Bucket: at.In(Beijing), Metrics: Rates(c)})
	}
	result.Summary = Rates(totals)
	return result
}
