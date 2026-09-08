package domain

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

var ErrInvalidAuction = errors.New("竞价需填写广告主标识、名称和有效的单次曝光出价；出价不能超过日预算")
var advertiserPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

type AuctionTerms struct {
	AdvertiserID   string `json:"advertiserId"`
	AdvertiserName string `json:"advertiserName"`
	BidFen         int64  `json:"bidFen"`
}

func NormalizeAuction(input *AuctionTerms, dailyBudgetFen int64) (*AuctionTerms, error) {
	if input == nil {
		return nil, nil
	}
	value := *input
	value.AdvertiserID = strings.ToLower(strings.TrimSpace(value.AdvertiserID))
	value.AdvertiserName = strings.TrimSpace(value.AdvertiserName)
	if !advertiserPattern.MatchString(value.AdvertiserID) || len([]rune(value.AdvertiserName)) < 2 || len([]rune(value.AdvertiserName)) > 128 || value.BidFen < 1 || value.BidFen > 1000000 || value.BidFen > dailyBudgetFen {
		return nil, ErrInvalidAuction
	}
	return &value, nil
}

func (v Version) Auction() *AuctionTerms {
	if v.auction == nil {
		return nil
	}
	value := *v.auction
	return &value
}

func NewAuctionVersion(number uint32, targeting TargetingRule, budget, legacyCost int64, frequency uint32, published time.Time, auction *AuctionTerms) (Version, error) {
	terms, err := NormalizeAuction(auction, budget)
	if err != nil {
		return Version{}, err
	}
	if terms != nil {
		legacyCost = terms.BidFen
	}
	v, err := NewVersion(number, targeting, budget, legacyCost, frequency, published)
	if err != nil {
		return Version{}, err
	}
	v.auction = terms
	return v, nil
}

func (c *Campaign) PublishAuction(rule TargetingRule, budget, legacyCost int64, frequency uint32, now time.Time, auction *AuctionTerms) error {
	terms, err := NormalizeAuction(auction, budget)
	if err != nil {
		return err
	}
	if terms != nil {
		legacyCost = terms.BidFen
	}
	if err := c.Publish(rule, budget, legacyCost, frequency, now); err != nil {
		return err
	}
	c.activeVersion.auction = terms
	return nil
}
