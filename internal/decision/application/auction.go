package application

import (
	"bytes"
	"crypto/sha256"
	"github.com/umi3730/adflow/internal/decision/domain"
	"sort"
	"time"
)

func auctionBefore(a, b domain.Candidate, requestID string) bool {
	if a.BidFen != b.BidFen {
		return a.BidFen > b.BidFen
	}
	// One advertiser cannot improve its tie probability by adding more plans.
	ah := sha256.Sum256([]byte(requestID + "\x00" + a.AdvertiserID))
	bh := sha256.Sum256([]byte(requestID + "\x00" + b.AdvertiserID))
	if comparison := bytes.Compare(ah[:], bh[:]); comparison != 0 {
		return comparison > 0
	}
	ah = sha256.Sum256([]byte(requestID + "\x00" + a.CampaignID))
	bh = sha256.Sum256([]byte(requestID + "\x00" + b.CampaignID))
	if comparison := bytes.Compare(ah[:], bh[:]); comparison != 0 {
		return comparison > 0
	}
	return a.CampaignID < b.CampaignID
}

// Only diagnose an empty ranking; preserve the reason in the original decision.
func unavailableCandidateReason(candidates []domain.Candidate, profile domain.Profile, now time.Time) domain.Reason {
	reason := domain.ReasonNoCandidate
	evaluator := domain.Evaluator{}
	for _, c := range candidates {
		if (!c.StartAt.IsZero() && now.Before(c.StartAt)) || (!c.EndAt.IsZero() && !now.Before(c.EndAt)) {
			continue
		}
		reason = domain.ReasonTargetingMiss
		if evaluator.Match(profile, c.Targeting) && len(c.CreativeIDs) == 0 {
			return domain.ReasonNoCreative
		}
	}
	return reason
}

func rankCandidates(candidates []domain.Candidate, profile domain.Profile, now time.Time, requestID string) ([]domain.Candidate, int) {
	representatives := make(map[string]domain.Candidate)
	legacy := make([]domain.Candidate, 0)
	evaluator := domain.Evaluator{}
	for _, c := range candidates {
		if (!c.StartAt.IsZero() && now.Before(c.StartAt)) || (!c.EndAt.IsZero() && !now.Before(c.EndAt)) || len(c.CreativeIDs) == 0 || !evaluator.Match(profile, c.Targeting) {
			continue
		}
		if c.BidFen <= 0 || c.AdvertiserID == "" {
			legacy = append(legacy, c)
			continue
		}
		previous, exists := representatives[c.AdvertiserID]
		if !exists || auctionBefore(c, previous, requestID) {
			representatives[c.AdvertiserID] = c
		}
	}
	ranked := make([]domain.Candidate, 0, len(representatives)+len(legacy))
	for _, c := range representatives {
		ranked = append(ranked, c)
	}
	sort.Slice(ranked, func(i, j int) bool { return auctionBefore(ranked[i], ranked[j], requestID) })
	sort.Slice(legacy, func(i, j int) bool { return legacy[i].CampaignID < legacy[j].CampaignID })
	return append(ranked, legacy...), len(representatives)
}
