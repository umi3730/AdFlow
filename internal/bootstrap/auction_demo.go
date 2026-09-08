package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	campaign "github.com/umi3730/adflow/internal/campaign/domain"
	decision "github.com/umi3730/adflow/internal/decision/domain"
	"time"
)

const AuctionDemoProfileID = "demo-auction-user"

func auctionFixtures(now time.Time) ([]demoFixture, []decision.Profile, error) {
	result := make([]demoFixture, 0, 3)
	for i, name := range []string{"星河游戏", "风铃互动", "北辰科技"} {
		id := fmt.Sprintf("demo-auction-%d", i+1)
		campaignName, err := campaign.NewName("竞价演示 · " + name)
		if err != nil {
			return nil, nil, err
		}
		slot, _ := campaign.NewSlotID(DemoSlotID)
		period, err := campaign.NewDeliveryPeriod(now.Add(-time.Hour), time.Date(2099, 12, 31, 23, 59, 59, 0, time.UTC))
		if err != nil {
			return nil, nil, err
		}
		c := campaign.NewCampaign(id, campaignName, slot, period)
		rule, _ := campaign.NewTargetingRule([]campaign.Condition{{Tag: "auction_demo"}}, nil, nil)
		terms := &campaign.AuctionTerms{AdvertiserID: fmt.Sprintf("studio-%d", i+1), AdvertiserName: name, BidFen: []int64{2, 5, 3}[i]}
		if err := c.PublishAuction(rule, 10000, 1, 100, now, terms); err != nil {
			return nil, nil, err
		}
		creative, err := campaign.NewCreative(id+"-creative", id, "竞价素材 · "+name, "", "https://adflow.invalid/demo-assets/bangdream/"+[]string{"anon", "tomori", "rana-2"}[i]+".webp", "https://example.com/game")
		if err != nil {
			return nil, nil, err
		}
		result = append(result, demoFixture{campaign: c, creative: creative})
	}
	return result, []decision.Profile{decision.NewProfile(AuctionDemoProfileID, []string{"auction_demo"}, map[string]string{"device": "android", "score": "80"})}, nil
}

// Separate versioned groups keep the original demo and deleted auction fixtures intact.
func (s *DemoInitializer) RunAuctionDemo(ctx context.Context, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	fixtures, profiles, err := auctionFixtures(now)
	if err != nil {
		return err
	}
	if s.options.PersistentCampaigns {
		_, err = s.mysqlGroup(ctx, "auction-campaigns-v1", func(tx *sql.Tx) error {
			for _, fixture := range fixtures {
				if err := insertCampaign(ctx, tx, fixture); err != nil {
					return err
				}
				terms := fixture.campaign.ActiveVersion().Auction()
				if _, err := tx.ExecContext(ctx, `UPDATE campaign_versions SET auction_terms = JSON_OBJECT('advertiserId', ?, 'advertiserName', ?, 'bidFen', ?) WHERE campaign_id = ? AND version = 1`, terms.AdvertiserID, terms.AdvertiserName, terms.BidFen, fixture.campaign.ID()); err != nil {
					return err
				}
			}
			return nil
		})
	} else if !s.auctionCampaignsDone {
		for _, fixture := range fixtures {
			// Reuse the normal memory inserts with the fixture's own reserved IDs.
			if err := s.options.Campaigns.Create(ctx, fixture.campaign); err != nil {
				return err
			}
			if err := s.options.Campaigns.CreateCreative(ctx, fixture.creative); err != nil {
				return err
			}
		}
		s.auctionCampaignsDone = true
	}
	if err != nil {
		return err
	}
	if s.options.PersistentProfiles {
		_, err = s.mysqlGroup(ctx, "auction-profiles-v1", func(tx *sql.Tx) error { return insertProfiles(ctx, tx, profiles) })
	} else if !s.auctionProfilesDone {
		err = s.installMemoryProfiles(ctx, profiles)
		if err == nil {
			s.auctionProfilesDone = true
		}
	}
	return err
}
