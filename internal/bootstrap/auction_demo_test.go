package bootstrap

import (
	campaign "github.com/zhanghaiyang/adflow/internal/campaign/domain"
	"testing"
	"time"
)

func TestAuctionDemoInstallsThreeAdvertisersAndPreservesDeletion(t *testing.T) {
	initializer, repo, profiles := memoryDemo(t)
	now := time.Now().UTC()
	if err := initializer.RunAuctionDemo(t.Context(), now); err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{"demo-auction-1", "demo-auction-2", "demo-auction-3"} {
		c, err := repo.FindByID(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		if c.ActiveVersion().Auction().BidFen != []int64{2, 5, 3}[i] {
			t.Fatal("wrong demo bid")
		}
	}
	if _, err := profiles.FindProfile(t.Context(), AuctionDemoProfileID); err != nil {
		t.Fatal(err)
	}
	c, _ := repo.FindByID(t.Context(), "demo-auction-2")
	revision := c.Revision()
	_ = c.Pause(now)
	_ = c.Delete()
	_ = repo.Save(t.Context(), c, revision)
	_ = profiles.DeleteProfile(t.Context(), AuctionDemoProfileID)
	if err := initializer.RunAuctionDemo(t.Context(), now); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.FindByID(t.Context(), "demo-auction-2"); err != campaign.ErrCampaignNotFound {
		t.Fatal("deleted demo restored")
	}
	if _, err := profiles.FindProfile(t.Context(), AuctionDemoProfileID); err == nil {
		t.Fatal("deleted profile restored")
	}
}
