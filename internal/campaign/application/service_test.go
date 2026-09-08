package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/umi3730/adflow/internal/campaign/adapter/memory"
	"github.com/umi3730/adflow/internal/campaign/domain"
)

func TestCreatePublishPauseResume(t *testing.T) {
	service := NewService(memory.NewRepository(), nil)
	created, err := service.Create(context.Background(), CreateCommand{
		Name: "Strategy Campaign", SlotID: "game-home-banner", StartAt: time.Now(), EndAt: time.Now().Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	published, err := service.Publish(context.Background(), PublishCommand{
		CampaignID: created.ID(), All: []domain.Condition{{Tag: "anime"}}, DailyBudgetFen: 10_000, ImpressionCostFen: 100, FrequencyLimit: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if published.Status() != domain.StatusActive {
		t.Fatalf("status = %s", published.Status())
	}
	if _, err := service.Pause(context.Background(), created.ID()); err != nil {
		t.Fatal(err)
	}
	resumed, err := service.Resume(context.Background(), created.ID())
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Status() != domain.StatusActive {
		t.Fatalf("status = %s", resumed.Status())
	}
}

func TestPublishRejectsUnknownCampaign(t *testing.T) {
	service := NewService(memory.NewRepository(), nil)
	_, err := service.Publish(context.Background(), PublishCommand{CampaignID: "missing"})
	if !errors.Is(err, domain.ErrCampaignNotFound) {
		t.Fatalf("Publish() error = %v", err)
	}
}
