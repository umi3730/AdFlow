package memory

import (
	"context"
	"github.com/zhanghaiyang/adflow/internal/decision/domain"
	"time"
)

type settlementReceipt struct {
	fingerprint string
	expiresAt   time.Time
}

func (r *Runtime) SettleImpression(ctx context.Context, settlement domain.Settlement) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	for token, receipt := range r.settlementReceipts {
		if !now.Before(receipt.expiresAt) {
			delete(r.settlementReceipts, token)
		}
	}
	d, fingerprint := settlement.Decision, settlement.Fingerprint()
	if receipt, exists := r.settlementReceipts[d.ReservationToken]; exists {
		if receipt.fingerprint == fingerprint {
			return nil
		}
		return domain.ErrSettlementUnavailable
	}
	budget, bok := r.budgetReservations[d.ReservationToken]
	frequency, fok := r.frequencyReservations[d.ReservationToken]
	if !bok || !fok || settlement.EventID == "" || budget.amount <= 0 || budget.amount != d.Pricing.PriceFen || !now.Before(budget.expiresAt) || !now.Before(frequency.expiresAt) {
		return domain.ErrSettlementUnavailable
	}
	// Reservation keys use the UTC reservation date; derive it from the stored key.
	if len(budget.key) < 11 || budget.key[10:] != ":"+d.CampaignID || frequency.key != budget.key+":"+d.UserID {
		return domain.ErrSettlementUnavailable
	}
	delete(r.budgetReservations, d.ReservationToken)
	delete(r.frequencyReservations, d.ReservationToken)
	r.settlementReceipts[d.ReservationToken] = settlementReceipt{fingerprint, now.Add(7 * 24 * time.Hour)}
	return nil
}
