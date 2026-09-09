package mysql

import (
	"context"
	"fmt"

	decisiondomain "github.com/umi3730/adflow/internal/decision/domain"
)

// Existing status-leading indexes bound the work. Terminal history (including
// RECONCILE) is excluded and cannot permanently shut down new decisions.
func (o *Outbox) ReadDecisionBacklog(ctx context.Context, settlementCap, outboxCap int64) (decisiondomain.AsyncBacklog, error) {
	if settlementCap <= 0 || outboxCap <= 0 {
		return decisiondomain.AsyncBacklog{}, fmt.Errorf("backlog caps must be positive")
	}
	var counts decisiondomain.AsyncBacklog
	err := o.db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM (SELECT 1 FROM event_settlements WHERE status IN ('PENDING', 'PROCESSING') LIMIT ?) AS active_settlements),
		(SELECT COUNT(*) FROM (SELECT 1 FROM event_outbox WHERE status IN ('SETTLING', 'PENDING', 'PROCESSING') LIMIT ?) AS active_outbox)`,
		settlementCap, outboxCap).Scan(&counts.Settlements, &counts.Outbox)
	return counts, err
}
