package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
)

var ErrSettlementUnavailable = errors.New("reservation settlement cannot be proven; reconciliation required")

// Settlement binds a durable impression to the exact winning reservation.
type Settlement struct {
	EventID  string
	Decision Result
}

func (s Settlement) Fingerprint() string {
	b, _ := json.Marshal([]any{s.EventID, s.Decision.RequestID, s.Decision.ReservationToken, s.Decision.UserID, s.Decision.CampaignID, s.Decision.CreativeID, s.Decision.Pricing.PriceFen})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

type ImpressionSettler interface {
	SettleImpression(context.Context, Settlement) error
}
