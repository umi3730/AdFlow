package httpadapter

import (
	"time"

	"github.com/zhanghaiyang/adflow/internal/campaign/domain"
)

type createRequest struct {
	Name    string    `json:"name" binding:"required,min=2,max=128"`
	SlotID  string    `json:"slotId" binding:"required,min=2,max=64"`
	StartAt time.Time `json:"startAt" binding:"required"`
	EndAt   time.Time `json:"endAt" binding:"required"`
}

type updateRequest struct {
	Name    string    `json:"name" binding:"required,min=2,max=128"`
	SlotID  string    `json:"slotId" binding:"required,min=2,max=64"`
	StartAt time.Time `json:"startAt" binding:"required"`
	EndAt   time.Time `json:"endAt" binding:"required"`
}

type publishRequest struct {
	Targeting         targetingRule `json:"targeting" binding:"required"`
	DailyBudgetFen    int64         `json:"dailyBudgetFen" binding:"required,gt=0"`
	ImpressionCostFen int64         `json:"impressionCostFen" binding:"required,gt=0"`
	FrequencyLimit    uint32        `json:"frequencyLimit" binding:"required,gte=1,lte=100"`
}

type createCreativeRequest struct {
	Title       string `json:"title" binding:"required,min=2,max=128"`
	Description string `json:"description" binding:"max=512"`
	ImageURL    string `json:"imageUrl" binding:"required,url"`
	LandingURL  string `json:"landingUrl" binding:"required,url"`
}

type targetingRule struct {
	All  []domain.Condition `json:"all"`
	Any  []domain.Condition `json:"any"`
	None []domain.Condition `json:"none"`
}

type campaignResponse struct {
	ID            string           `json:"id"`
	Name          string           `json:"name"`
	SlotID        string           `json:"slotId"`
	StartAt       time.Time        `json:"startAt"`
	EndAt         time.Time        `json:"endAt"`
	Status        domain.Status    `json:"status"`
	Revision      uint64           `json:"revision"`
	ActiveVersion *versionResponse `json:"activeVersion,omitempty"`
}

type versionResponse struct {
	Number            uint32               `json:"number"`
	Targeting         domain.TargetingRule `json:"targeting"`
	DailyBudgetFen    int64                `json:"dailyBudgetFen"`
	ImpressionCostFen int64                `json:"impressionCostFen"`
	FrequencyLimit    uint32               `json:"frequencyLimit"`
	PublishedAt       time.Time            `json:"publishedAt"`
}

type creativeResponse struct {
	ID          string                `json:"id"`
	CampaignID  string                `json:"campaignId"`
	Title       string                `json:"title"`
	Description string                `json:"description"`
	ImageURL    string                `json:"imageUrl"`
	LandingURL  string                `json:"landingUrl"`
	Status      domain.CreativeStatus `json:"status"`
	Revision    uint64                `json:"revision"`
}

func toResponse(campaign *domain.Campaign) campaignResponse {
	response := campaignResponse{
		ID:       campaign.ID(),
		Name:     string(campaign.Name()),
		SlotID:   string(campaign.SlotID()),
		StartAt:  campaign.Period().Start(),
		EndAt:    campaign.Period().End(),
		Status:   campaign.Status(),
		Revision: campaign.Revision(),
	}
	if version := campaign.ActiveVersion(); version != nil {
		response.ActiveVersion = &versionResponse{
			Number:            version.Number(),
			Targeting:         version.Targeting(),
			DailyBudgetFen:    version.DailyBudget().Amount(),
			ImpressionCostFen: version.ImpressionCost().Amount(),
			FrequencyLimit:    version.FrequencyLimit(),
			PublishedAt:       version.PublishedAt(),
		}
	}
	return response
}

func toCreativeResponse(creative *domain.Creative) creativeResponse {
	return creativeResponse{
		ID: creative.ID(), CampaignID: creative.CampaignID(), Title: creative.Title(), Description: creative.Description(),
		ImageURL: creative.ImageURL(), LandingURL: creative.LandingURL(), Status: creative.Status(), Revision: creative.Revision(),
	}
}
