package mysql

import (
	"context"
	"database/sql"
	"github.com/umi3730/adflow/internal/reporting/domain"
	"time"
)

type Reader struct{ db *sql.DB }

func NewReader(db *sql.DB) *Reader { return &Reader{db: db} }

// ReadDeliveryRows groups the deduplicated event facts. Spend comes exclusively
// from the immutable, confirmed impression settlement, never the current bid.
func (r *Reader) ReadDeliveryRows(ctx context.Context, f domain.Filter) ([]domain.Row, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}
	format := "%Y-%m-%d %H:00:00"
	if f.Granularity == "day" {
		format = "%Y-%m-%d 00:00:00"
	}
	query := `SELECT DATE_FORMAT(DATE_ADD(e.occurred_at, INTERVAL 8 HOUR), ?),
 SUM(e.event_type='impression'), SUM(e.event_type='click'), SUM(e.event_type='conversion'),
 COALESCE(SUM(CASE WHEN e.event_type='impression' AND s.status='SETTLED' AND s.event_id=e.event_id
 THEN CAST(JSON_UNQUOTE(JSON_EXTRACT(s.decision_snapshot,'$.Pricing.priceFen')) AS SIGNED) ELSE 0 END),0),
 COALESCE(SUM(CASE WHEN e.event_type='conversion' THEN e.value_fen ELSE 0 END),0),
 SUM(CASE WHEN e.event_type='impression' AND (s.status IS NULL OR s.status<>'SETTLED' OR s.event_id<>e.event_id
 OR JSON_EXTRACT(s.decision_snapshot,'$.Pricing.priceFen') IS NULL) THEN 1 ELSE 0 END)
 FROM processed_events e LEFT JOIN event_settlements s ON s.request_id=e.request_id
 WHERE e.occurred_at>=? AND e.occurred_at<?`
	args := []any{format, f.From.UTC(), f.To.UTC()}
	if f.CampaignID != "" {
		query += " AND e.campaign_id=?"
		args = append(args, f.CampaignID)
	}
	query += " GROUP BY 1 ORDER BY 1"
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.Row{}
	for rows.Next() {
		var row domain.Row
		var bucket string
		if err := rows.Scan(&bucket, &row.Impressions, &row.Clicks, &row.Conversions, &row.SpendFen, &row.ValueFen, &row.UnpricedImpressions); err != nil {
			return nil, err
		}
		row.Bucket, err = time.ParseInLocation("2006-01-02 15:04:05", bucket, domain.Beijing)
		if err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}
