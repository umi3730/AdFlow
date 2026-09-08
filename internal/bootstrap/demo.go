// Package bootstrap installs the bundled demonstration data before HTTP serving.
package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	campaign "github.com/umi3730/adflow/internal/campaign/domain"
	decision "github.com/umi3730/adflow/internal/decision/domain"
	"github.com/umi3730/adflow/internal/profile/schema"
)

const (
	DemoCampaignID  = "demo-campaign-v1"
	DemoCreativeID  = "demo-creative-v1"
	DemoSlotID      = "game-home-banner"
	campaignSeedKey = "demo-campaign-v1"
	profileSeedKey  = "demo-profiles-v1"
)

func DemoProfileIDs() []string {
	return []string{"demo-user-match", "demo-user-excluded", "demo-user-miss"}
}

type CampaignRepository interface {
	campaign.Repository
	campaign.CreativeRepository
}

type DemoOptions struct {
	DB                  *sql.DB
	Campaigns           CampaignRepository
	Profiles            decision.ProfileStore
	PersistentCampaigns bool
	PersistentProfiles  bool
}

type DemoResult struct {
	CampaignsInstalled bool
	ProfilesInstalled  bool
}

// DemoInitializer is tied to one set of repositories. Keep it for that set's
// lifetime: its memory flags have the same lifetime as the in-memory data.
type DemoInitializer struct {
	options              DemoOptions
	mu                   sync.Mutex
	campaignsDone        bool
	profilesDone         bool
	auctionCampaignsDone bool
	auctionProfilesDone  bool
}

func NewDemoInitializer(options DemoOptions) (*DemoInitializer, error) {
	if (!options.PersistentCampaigns && options.Campaigns == nil) || (!options.PersistentProfiles && options.Profiles == nil) {
		return nil, errors.New("demo data requires its in-memory repositories")
	}
	if (options.PersistentCampaigns || options.PersistentProfiles) && options.DB == nil {
		return nil, errors.New("persistent demo data requires a database")
	}
	return &DemoInitializer{options: options}, nil
}

// Run never repairs an installed fixture. User edits and deletions are final.
// Each persistent group is atomic; an error must prevent the API from serving.
func (s *DemoInitializer) Run(ctx context.Context, now time.Time) (DemoResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result DemoResult
	if err := ctx.Err(); err != nil {
		return result, err
	}
	fixture, err := newDemoFixture(now)
	if err != nil {
		return result, err
	}
	if s.options.PersistentCampaigns {
		result.CampaignsInstalled, err = s.mysqlGroup(ctx, campaignSeedKey, func(tx *sql.Tx) error {
			return insertCampaign(ctx, tx, fixture)
		})
	} else if !s.campaignsDone {
		err = s.installMemoryCampaign(ctx, fixture)
		if err == nil {
			s.campaignsDone = true
			result.CampaignsInstalled = true
		}
	}
	if err != nil {
		return result, fmt.Errorf("initialize demo campaign: %w", err)
	}
	if s.options.PersistentProfiles {
		result.ProfilesInstalled, err = s.mysqlGroup(ctx, profileSeedKey, func(tx *sql.Tx) error {
			return insertProfiles(ctx, tx, fixture.profiles)
		})
	} else if !s.profilesDone {
		err = s.installMemoryProfiles(ctx, fixture.profiles)
		if err == nil {
			s.profilesDone = true
			result.ProfilesInstalled = true
		}
	}
	if err != nil {
		return result, fmt.Errorf("initialize demo profiles: %w", err)
	}
	return result, nil
}

type demoFixture struct {
	campaign *campaign.Campaign
	creative *campaign.Creative
	profiles []decision.Profile
}

func newDemoFixture(now time.Time) (demoFixture, error) {
	var fixture demoFixture
	name, err := campaign.NewName("演示计划 · 游戏首页")
	if err != nil {
		return fixture, err
	}
	slot, err := campaign.NewSlotID(DemoSlotID)
	if err != nil {
		return fixture, err
	}
	period, err := campaign.NewDeliveryPeriod(now.UTC().Add(-24*time.Hour), time.Date(2099, 12, 31, 23, 59, 59, 0, time.UTC))
	if err != nil {
		return fixture, err
	}
	rule, err := campaign.NewTargetingRule([]campaign.Condition{{Tag: "adflow_demo"}}, nil, []campaign.Condition{{Tag: "demo_excluded"}})
	if err != nil {
		return fixture, err
	}
	fixture.campaign = campaign.NewCampaign(DemoCampaignID, name, slot, period)
	if err := fixture.campaign.Publish(rule, 10_000, 1, 100, now); err != nil {
		return fixture, err
	}
	fixture.creative, err = campaign.NewCreative(DemoCreativeID, DemoCampaignID, "演示素材 · 游戏首页", "内置演示素材，可禁用和删除。", "https://adflow.invalid/demo-assets/bangdream/anon.webp", "https://example.com/game")
	if err != nil {
		return fixture, err
	}
	ids := DemoProfileIDs()
	fixture.profiles = []decision.Profile{
		decision.NewProfile(ids[0], []string{"adflow_demo", "gaming_interest"}, map[string]string{"device": "android", "score": "85"}),
		decision.NewProfile(ids[1], []string{"adflow_demo", "demo_excluded"}, map[string]string{"device": "ios", "score": "80"}),
		decision.NewProfile(ids[2], []string{"tech_interest"}, map[string]string{"device": "android", "score": "60"}),
	}
	for _, profile := range fixture.profiles {
		if err := schema.ValidateFields(profile.Fields); err != nil {
			return fixture, err
		}
	}
	return fixture, nil
}

func (s *DemoInitializer) installMemoryCampaign(ctx context.Context, fixture demoFixture) error {
	if _, err := s.options.Campaigns.FindByID(ctx, DemoCampaignID); !errors.Is(err, campaign.ErrCampaignNotFound) {
		if err != nil {
			return err
		}
		return fmt.Errorf("demo campaign ID %s already exists", DemoCampaignID)
	}
	if _, err := s.options.Campaigns.FindCreativeByID(ctx, DemoCreativeID); !errors.Is(err, campaign.ErrCreativeNotFound) {
		if err != nil {
			return err
		}
		return fmt.Errorf("demo creative ID %s already exists", DemoCreativeID)
	}
	if err := s.options.Campaigns.Create(ctx, fixture.campaign); err != nil {
		return err
	}
	return s.options.Campaigns.CreateCreative(ctx, fixture.creative)
}

func (s *DemoInitializer) installMemoryProfiles(ctx context.Context, profiles []decision.Profile) error {
	// PutProfile is an upsert: preflight every reserved ID before writing any.
	// Startup owns these memory stores exclusively, so there is no concurrent UI.
	for _, profile := range profiles {
		if _, err := s.options.Profiles.FindProfile(ctx, profile.UserID); !errors.Is(err, decision.ErrProfileNotFound) {
			if err != nil {
				return err
			}
			return fmt.Errorf("demo profile ID %s already exists", profile.UserID)
		}
	}
	for _, profile := range profiles {
		if err := s.options.Profiles.PutProfile(ctx, profile); err != nil {
			return err
		}
	}
	return nil
}
