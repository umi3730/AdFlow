package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/zhanghaiyang/adflow/internal/campaign/domain"
)

type EventPublisher interface {
	Publish(context.Context, []domain.Event) error
}

type Service struct {
	repository domain.Repository
	publisher  EventPublisher
	now        func() time.Time
	newID      func() string
}

func NewService(repository domain.Repository, publisher EventPublisher) *Service {
	return &Service{repository: repository, publisher: publisher, now: time.Now, newID: newID}
}

func (s *Service) Create(ctx context.Context, command CreateCommand) (*domain.Campaign, error) {
	name, err := domain.NewName(command.Name)
	if err != nil {
		return nil, err
	}
	slotID, err := domain.NewSlotID(command.SlotID)
	if err != nil {
		return nil, err
	}
	period, err := domain.NewDeliveryPeriod(command.StartAt, command.EndAt)
	if err != nil {
		return nil, err
	}
	campaign := domain.NewCampaign(s.newID(), name, slotID, period)
	if err := s.repository.Create(ctx, campaign); err != nil {
		return nil, err
	}
	return campaign.Clone(), nil
}

func (s *Service) Get(ctx context.Context, id string) (*domain.Campaign, error) {
	campaign, err := s.repository.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return campaign.Clone(), nil
}

func (s *Service) Update(ctx context.Context, command UpdateCommand) (*domain.Campaign, error) {
	campaign, err := s.repository.FindByID(ctx, command.CampaignID)
	if err != nil {
		return nil, err
	}
	name, err := domain.NewName(command.Name)
	if err != nil {
		return nil, err
	}
	slotID, err := domain.NewSlotID(command.SlotID)
	if err != nil {
		return nil, err
	}
	period, err := domain.NewDeliveryPeriod(command.StartAt, command.EndAt)
	if err != nil {
		return nil, err
	}
	expectedRevision := campaign.Revision()
	if err := campaign.UpdateDraft(name, slotID, period); err != nil {
		return nil, err
	}
	if err := s.repository.Save(ctx, campaign, expectedRevision); err != nil {
		return nil, err
	}
	return campaign.Clone(), nil
}

func (s *Service) List(ctx context.Context, filter domain.ListFilter) ([]*domain.Campaign, error) {
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 20
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	return s.repository.List(ctx, filter)
}

func (s *Service) Publish(ctx context.Context, command PublishCommand) (*domain.Campaign, error) {
	campaign, err := s.repository.FindByID(ctx, command.CampaignID)
	if err != nil {
		return nil, err
	}
	expectedRevision := campaign.Revision()
	rule, err := domain.NewTargetingRule(command.All, command.Any, command.None)
	if err != nil {
		return nil, err
	}
	if err := campaign.PublishAuction(rule, command.DailyBudgetFen, command.ImpressionCostFen, command.FrequencyLimit, s.now(), command.Auction); err != nil {
		return nil, err
	}
	return s.saveAndPublish(ctx, campaign, expectedRevision)
}

func (s *Service) Pause(ctx context.Context, id string) (*domain.Campaign, error) {
	return s.mutate(ctx, id, func(campaign *domain.Campaign) error { return campaign.Pause(s.now()) })
}

func (s *Service) Delete(ctx context.Context, id string) error {
	campaign, err := s.repository.FindByID(ctx, id)
	if err != nil {
		return err
	}
	revision := campaign.Revision()
	if err := campaign.Delete(); err != nil {
		return err
	}
	return s.repository.Save(ctx, campaign, revision)
}

func (s *Service) Resume(ctx context.Context, id string) (*domain.Campaign, error) {
	return s.mutate(ctx, id, func(campaign *domain.Campaign) error { return campaign.Resume(s.now()) })
}

func (s *Service) mutate(ctx context.Context, id string, mutation func(*domain.Campaign) error) (*domain.Campaign, error) {
	campaign, err := s.repository.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	expectedRevision := campaign.Revision()
	if err := mutation(campaign); err != nil {
		return nil, err
	}
	return s.saveAndPublish(ctx, campaign, expectedRevision)
}

func (s *Service) saveAndPublish(ctx context.Context, campaign *domain.Campaign, expectedRevision uint64) (*domain.Campaign, error) {
	if err := s.repository.Save(ctx, campaign, expectedRevision); err != nil {
		return nil, err
	}
	if s.publisher != nil {
		if err := s.publisher.Publish(ctx, campaign.PullEvents()); err != nil {
			return nil, err
		}
	}
	return campaign.Clone(), nil
}

func newID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(value[:])
}

type CreativeService struct {
	campaigns domain.Repository
	creatives domain.CreativeRepository
	newID     func() string
}

func NewCreativeService(campaigns domain.Repository, creatives domain.CreativeRepository) *CreativeService {
	return &CreativeService{campaigns: campaigns, creatives: creatives, newID: newID}
}

func (s *CreativeService) Create(ctx context.Context, command CreateCreativeCommand) (*domain.Creative, error) {
	if _, err := s.campaigns.FindByID(ctx, command.CampaignID); err != nil {
		return nil, err
	}
	creative, err := domain.NewCreative(s.newID(), command.CampaignID, command.Title, command.Description, command.ImageURL, command.LandingURL)
	if err != nil {
		return nil, err
	}
	if err := s.creatives.CreateCreative(ctx, creative); err != nil {
		return nil, err
	}
	return creative.Clone(), nil
}

func (s *CreativeService) List(ctx context.Context, campaignID string) ([]*domain.Creative, error) {
	if _, err := s.campaigns.FindByID(ctx, campaignID); err != nil {
		return nil, err
	}
	return s.creatives.ListCreativesByCampaign(ctx, campaignID)
}

func (s *CreativeService) Disable(ctx context.Context, campaignID, creativeID string) (*domain.Creative, error) {
	return s.changeAvailability(ctx, campaignID, creativeID, false)
}

func (s *CreativeService) Enable(ctx context.Context, campaignID, creativeID string) (*domain.Creative, error) {
	return s.changeAvailability(ctx, campaignID, creativeID, true)
}

func (s *CreativeService) changeAvailability(ctx context.Context, campaignID, creativeID string, enable bool) (*domain.Creative, error) {
	if _, err := s.campaigns.FindByID(ctx, campaignID); err != nil {
		return nil, err
	}
	creative, err := s.creatives.FindCreativeByID(ctx, creativeID)
	if err != nil {
		return nil, err
	}
	if creative.CampaignID() != campaignID {
		return nil, domain.ErrCreativeNotFound
	}
	expectedRevision := creative.Revision()
	if enable {
		err = creative.Enable()
	} else {
		err = creative.Disable()
	}
	if err != nil {
		return nil, err
	}
	if err := s.creatives.SaveCreative(ctx, creative, expectedRevision); err != nil {
		return nil, err
	}
	return creative.Clone(), nil
}

func (s *CreativeService) Delete(ctx context.Context, campaignID, creativeID string) error {
	if _, err := s.campaigns.FindByID(ctx, campaignID); err != nil {
		return err
	}
	creative, err := s.creatives.FindCreativeByID(ctx, creativeID)
	if err != nil {
		return err
	}
	if creative.CampaignID() != campaignID {
		return domain.ErrCreativeNotFound
	}
	revision := creative.Revision()
	if err := creative.Delete(); err != nil {
		return err
	}
	return s.creatives.SaveCreative(ctx, creative, revision)
}
