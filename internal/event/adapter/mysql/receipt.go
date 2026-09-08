package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/zhanghaiyang/adflow/internal/event/domain"
)

func (o *Outbox) FindImpressionReceipt(ctx context.Context, requestID string) (domain.ImpressionReceipt, bool, error) {
	var receipt domain.ImpressionReceipt
	var payload []byte
	err := o.db.QueryRowContext(ctx, `SELECT o.payload, s.accepted_at FROM event_settlements s JOIN event_outbox o ON o.event_id = s.event_id WHERE s.request_id = ?`, requestID).Scan(&payload, &receipt.AcceptedAt)
	if err == sql.ErrNoRows {
		return receipt, false, nil
	}
	if err != nil {
		return receipt, false, err
	}
	if err := json.Unmarshal(payload, &receipt.Event); err != nil {
		return receipt, false, err
	}
	return receipt, true, nil
}
