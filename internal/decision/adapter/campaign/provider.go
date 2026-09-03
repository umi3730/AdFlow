package campaign

import (
	"context"
	"time"

	campaigndomain "github.com/zhanghaiyang/adflow/internal/campaign/domain"
	decisiondomain "github.com/zhanghaiyang/adflow/internal/decision/domain"
)

type Provider struct {
	campaigns campaigndomain.Repository
	creatives campaigndomain.CreativeRepository
}

func NewProvider(campaigns campaigndomain.Repository, creatives campaigndomain.CreativeRepository) *Provider {
	return &Provider{campaigns: campaigns, creatives: creatives}
}

func (p *Provider) ActiveCandidates(ctx context.Context, slotID string, now time.Time) ([]decisiondomain.Candidate, error) {
	status := campaigndomain.StatusActive
	campaigns, err := p.campaigns.List(ctx, campaigndomain.ListFilter{Status: &status, Limit: 100})
	if err != nil {
		return nil, err
	}
	result := make([]decisiondomain.Candidate, 0, len(campaigns))
	for _, campaign := range campaigns {
		if string(campaign.SlotID()) != slotID || now.Before(campaign.Period().Start()) || !now.Before(campaign.Period().End()) {
			continue
		}
		version := campaign.ActiveVersion()
		if version == nil {
			continue
		}
		creatives, err := p.creatives.ListCreativesByCampaign(ctx, campaign.ID())
		if err != nil {
			return nil, err
		}
		creativeIDs := make([]string, 0, len(creatives))
		for _, creative := range creatives {
			if creative.Status() == campaigndomain.CreativeActive {
				creativeIDs = append(creativeIDs, creative.ID())
			}
		}
		result = append(result, decisiondomain.Candidate{
			CampaignID: campaign.ID(), CreativeIDs: creativeIDs, Targeting: mapRule(version.Targeting()),
			DailyBudgetFen: version.DailyBudget().Amount(), ImpressionCostFen: version.ImpressionCost().Amount(),
			FrequencyLimit: version.FrequencyLimit(), StartAt: campaign.Period().Start(), EndAt: campaign.Period().End(),
		})
	}
	return result, nil
}

func mapRule(rule campaigndomain.TargetingRule) decisiondomain.TargetingRule {
	return decisiondomain.TargetingRule{
		All: mapConditions(rule.All), Any: mapConditions(rule.Any), None: mapConditions(rule.None),
	}
}

func mapConditions(input []campaigndomain.Condition) []decisiondomain.Condition {
	result := make([]decisiondomain.Condition, 0, len(input))
	for _, condition := range input {
		result = append(result, decisiondomain.Condition{
			Tag: condition.Tag, Field: condition.Field, Op: condition.Op, Value: condition.Value,
		})
	}
	return result
}
