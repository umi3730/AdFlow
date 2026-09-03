package application

import (
	"time"

	"github.com/zhanghaiyang/adflow/internal/campaign/domain"
)

type CreateCommand struct {
	Name    string
	SlotID  string
	StartAt time.Time
	EndAt   time.Time
}

type UpdateCommand struct {
	CampaignID string
	Name       string
	SlotID     string
	StartAt    time.Time
	EndAt      time.Time
}

type PublishCommand struct {
	CampaignID        string
	All               []domain.Condition
	Any               []domain.Condition
	None              []domain.Condition
	DailyBudgetFen    int64
	ImpressionCostFen int64
	FrequencyLimit    uint32
}

type CreateCreativeCommand struct {
	CampaignID  string
	Title       string
	Description string
	ImageURL    string
	LandingURL  string
}
