package domain

import "context"

type Repository interface {
	Create(context.Context, *Campaign) error
	Save(context.Context, *Campaign, uint64) error
	FindByID(context.Context, string) (*Campaign, error)
	List(context.Context, ListFilter) ([]*Campaign, error)
}

type ListFilter struct {
	Status *Status
	SlotID *SlotID
	Limit  int
	Offset int
	// AfterID selects stable ascending-ID pagination for internal full scans.
	// A non-nil empty string starts the scan; normal list ordering is unchanged.
	AfterID *string
}

type CreativeRepository interface {
	CreateCreative(context.Context, *Creative) error
	SaveCreative(context.Context, *Creative, uint64) error
	FindCreativeByID(context.Context, string) (*Creative, error)
	ListCreativesByCampaign(context.Context, string) ([]*Creative, error)
	ListActiveCreativeIDsByCampaigns(context.Context, []string) (map[string][]string, error)
}
