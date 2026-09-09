package memory

import (
	"context"
	ed "github.com/umi3730/adflow/internal/event/domain"
	rd "github.com/umi3730/adflow/internal/reporting/domain"
	"time"
)

func (s *Store) ReadDeliveryRows(ctx context.Context, f rd.Filter) ([]rd.Row, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	buckets := map[time.Time]rd.Counts{}
	for _, e := range s.events {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if e.OccurredAt.Before(f.From) || !e.OccurredAt.Before(f.To) || (f.CampaignID != "" && e.CampaignID != f.CampaignID) {
			continue
		}
		key := rd.BucketStart(e.OccurredAt, f.Granularity)
		c := buckets[key]
		switch e.Type {
		case ed.Impression:
			c.Impressions++
			if submission, ok := s.submissions[e.EventID]; ok && submission.Processed && submission.Decision.Pricing.PriceFen > 0 {
				c.SpendFen += submission.Decision.Pricing.PriceFen
			} else {
				c.UnpricedImpressions++
			}
		case ed.Click:
			c.Clicks++
		case ed.Conversion:
			c.Conversions++
			c.ValueFen += e.ValueFen
		}
		buckets[key] = c
	}
	result := make([]rd.Row, 0, len(buckets))
	for at, c := range buckets {
		result = append(result, rd.Row{Bucket: at, Counts: c})
	}
	return result, nil
}
