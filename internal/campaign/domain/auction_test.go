package domain

import (
	"testing"
	"time"
)

func TestAuctionTermsValidateAndPublishedVersionsRemainIsolated(t *testing.T) {
	name, _ := NewName("auction test")
	slot, _ := NewSlotID("slot")
	now := time.Now().UTC()
	period, _ := NewDeliveryPeriod(now, now.Add(time.Hour))
	c := NewCampaign("campaign", name, slot, period)
	rule, _ := NewTargetingRule([]Condition{{Tag: "auction"}}, nil, nil)
	for _, terms := range []*AuctionTerms{{AdvertiserID: "a", AdvertiserName: "甲公司", BidFen: 0}, {AdvertiserID: "bad id", AdvertiserName: "甲公司", BidFen: 5}, {AdvertiserID: "a", AdvertiserName: "甲公司", BidFen: 101}} {
		if err := c.PublishAuction(rule, 100, 1, 3, now, terms); err == nil {
			t.Fatal("invalid bid accepted")
		}
		if c.Status() != StatusDraft || c.Revision() != 1 {
			t.Fatal("failed publish mutated campaign")
		}
	}
	terms := &AuctionTerms{AdvertiserID: "STUDIO-A", AdvertiserName: "甲公司", BidFen: 5}
	if err := c.PublishAuction(rule, 100, 1, 3, now, terms); err != nil {
		t.Fatal(err)
	}
	old := c.ActiveVersion()
	terms.BidFen = 99
	exposed := old.Auction()
	exposed.BidFen = 88
	if old.Auction().BidFen != 5 || old.Auction().AdvertiserID != "studio-a" || old.ImpressionCost().Amount() != 5 {
		t.Fatal("auction snapshot leaked")
	}
	_ = c.Pause(now)
	if err := c.Publish(rule, 100, 2, 3, now); err != nil {
		t.Fatal(err)
	}
	if c.ActiveVersion().Auction() != nil || old.Auction().BidFen != 5 {
		t.Fatal("legacy/auction versions mixed")
	}
}
