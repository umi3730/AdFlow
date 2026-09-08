package campaign

import (
	"context"
	"errors"
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

const candidatePageSize = 100

func (p *Provider) ActiveCandidates(ctx context.Context, slotID string, now time.Time) ([]decisiondomain.Candidate, error) {
	status := campaigndomain.StatusActive
	slot, err := campaigndomain.NewSlotID(slotID)
	if err != nil {
		return nil, err
	}
	result := make([]decisiondomain.Candidate, 0)
	seen := make(map[string]struct{})
	cursor := ""
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		page, err := p.campaigns.List(ctx, campaigndomain.ListFilter{Status: &status, SlotID: &slot, Limit: candidatePageSize, AfterID: &cursor})
		if err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		eligible := make([]*campaigndomain.Campaign, 0, len(page))
		campaignIDs := make([]string, 0, len(page))
		for _, campaign := range page {
			if _, exists := seen[campaign.ID()]; exists {
				continue
			}
			if now.Before(campaign.Period().Start()) || !now.Before(campaign.Period().End()) || campaign.ActiveVersion() == nil {
				continue
			}
			seen[campaign.ID()] = struct{}{}
			eligible = append(eligible, campaign)
			campaignIDs = append(campaignIDs, campaign.ID())
		}
		// One bounded creative query per page, rather than one query per plan.
		if len(eligible) > 0 {
			creativeIDs, err := p.creatives.ListActiveCreativeIDsByCampaigns(ctx, campaignIDs)
			if err != nil {
				return nil, err
			}
			for _, campaign := range eligible {
				version := campaign.ActiveVersion()
				terms := version.Auction()
				var advertiserID, advertiserName string
				var bid int64
				if terms != nil {
					advertiserID, advertiserName, bid = terms.AdvertiserID, terms.AdvertiserName, terms.BidFen
				}
				result = append(result, decisiondomain.Candidate{
					AdvertiserID: advertiserID, AdvertiserName: advertiserName, BidFen: bid, Version: version.Number(),
					CampaignID: campaign.ID(), CreativeIDs: creativeIDs[campaign.ID()], Targeting: mapRule(version.Targeting()),
					DailyBudgetFen: version.DailyBudget().Amount(), ImpressionCostFen: version.ImpressionCost().Amount(),
					FrequencyLimit: version.FrequencyLimit(), StartAt: campaign.Period().Start(), EndAt: campaign.Period().End(),
				})
			}
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// Move past the last raw ID even when every row was ineligible. Unlike
		// OFFSET, deleting or inserting earlier rows cannot shift the next page.
		if len(page) < candidatePageSize {
			return result, nil
		}
		next := page[len(page)-1].ID()
		if next <= cursor {
			return nil, errors.New("candidate repository cursor did not advance")
		}
		cursor = next
	}
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
