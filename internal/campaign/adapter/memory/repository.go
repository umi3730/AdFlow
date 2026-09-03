package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/zhanghaiyang/adflow/internal/campaign/domain"
)

type Repository struct {
	mu        sync.RWMutex
	campaigns map[string]*domain.Campaign
	creatives map[string]*domain.Creative
}

func NewRepository() *Repository {
	return &Repository{campaigns: make(map[string]*domain.Campaign), creatives: make(map[string]*domain.Creative)}
}

func (r *Repository) Create(_ context.Context, campaign *domain.Campaign) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.campaigns[campaign.ID()]; exists {
		return domain.ErrConcurrentMutation
	}
	r.campaigns[campaign.ID()] = campaign.Clone()
	return nil
}

func (r *Repository) Save(_ context.Context, campaign *domain.Campaign, expectedRevision uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, exists := r.campaigns[campaign.ID()]
	if !exists {
		return domain.ErrCampaignNotFound
	}
	if current.Revision() != expectedRevision {
		return domain.ErrConcurrentMutation
	}
	r.campaigns[campaign.ID()] = campaign.Clone()
	return nil
}

func (r *Repository) FindByID(_ context.Context, id string) (*domain.Campaign, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	campaign, exists := r.campaigns[id]
	if !exists {
		return nil, domain.ErrCampaignNotFound
	}
	return campaign.Clone(), nil
}

func (r *Repository) List(_ context.Context, filter domain.ListFilter) ([]*domain.Campaign, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*domain.Campaign, 0, len(r.campaigns))
	for _, campaign := range r.campaigns {
		if filter.Status != nil && campaign.Status() != *filter.Status {
			continue
		}
		result = append(result, campaign.Clone())
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID() < result[j].ID() })
	start := min(filter.Offset, len(result))
	end := min(start+filter.Limit, len(result))
	return result[start:end], nil
}

func (r *Repository) CreateCreative(_ context.Context, creative *domain.Creative) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.creatives[creative.ID()]; exists {
		return domain.ErrConcurrentMutation
	}
	r.creatives[creative.ID()] = creative.Clone()
	return nil
}

func (r *Repository) SaveCreative(_ context.Context, creative *domain.Creative, expectedRevision uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, exists := r.creatives[creative.ID()]
	if !exists {
		return domain.ErrCreativeNotFound
	}
	if current.Revision() != expectedRevision {
		return domain.ErrConcurrentMutation
	}
	r.creatives[creative.ID()] = creative.Clone()
	return nil
}

func (r *Repository) FindCreativeByID(_ context.Context, id string) (*domain.Creative, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	creative, exists := r.creatives[id]
	if !exists {
		return nil, domain.ErrCreativeNotFound
	}
	return creative.Clone(), nil
}

func (r *Repository) ListCreativesByCampaign(_ context.Context, campaignID string) ([]*domain.Creative, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*domain.Creative, 0)
	for _, creative := range r.creatives {
		if creative.CampaignID() == campaignID {
			result = append(result, creative.Clone())
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID() < result[j].ID() })
	return result, nil
}
