package domain

import "time"

type Version struct {
	number         uint32
	targeting      TargetingRule
	dailyBudget    Money
	impressionCost Money
	frequencyLimit uint32
	publishedAt    time.Time
}

func NewVersion(number uint32, targeting TargetingRule, dailyBudgetFen, impressionCostFen int64, frequencyLimit uint32, publishedAt time.Time) (Version, error) {
	budget, err := NewCNYFen(dailyBudgetFen)
	if err != nil {
		return Version{}, err
	}
	cost, err := newImpressionCost(impressionCostFen)
	if err != nil {
		return Version{}, err
	}
	if budget.Amount() < cost.Amount() {
		return Version{}, ErrBudgetBelowCost
	}
	if frequencyLimit == 0 || frequencyLimit > 100 {
		return Version{}, ErrInvalidFrequency
	}
	if publishedAt.IsZero() {
		publishedAt = time.Now().UTC()
	}
	return Version{
		number:         number,
		targeting:      targeting.Clone(),
		dailyBudget:    budget,
		impressionCost: cost,
		frequencyLimit: frequencyLimit,
		publishedAt:    publishedAt.UTC(),
	}, nil
}

func (v Version) Number() uint32           { return v.number }
func (v Version) Targeting() TargetingRule { return v.targeting.Clone() }
func (v Version) DailyBudget() Money       { return v.dailyBudget }
func (v Version) ImpressionCost() Money    { return v.impressionCost }
func (v Version) FrequencyLimit() uint32   { return v.frequencyLimit }
func (v Version) PublishedAt() time.Time   { return v.publishedAt }

type Campaign struct {
	id            string
	name          Name
	slotID        SlotID
	period        DeliveryPeriod
	status        Status
	activeVersion *Version
	revision      uint64
	events        []Event
}

func NewCampaign(id string, name Name, slotID SlotID, period DeliveryPeriod) *Campaign {
	return &Campaign{id: id, name: name, slotID: slotID, period: period, status: StatusDraft, revision: 1}
}

func Rehydrate(id string, name Name, slotID SlotID, period DeliveryPeriod, status Status, activeVersion *Version, revision uint64) *Campaign {
	return &Campaign{id: id, name: name, slotID: slotID, period: period, status: status, activeVersion: cloneVersion(activeVersion), revision: revision}
}

func (c *Campaign) Publish(rule TargetingRule, dailyBudgetFen, impressionCostFen int64, frequencyLimit uint32, now time.Time) error {
	if c.status != StatusDraft && c.status != StatusPaused {
		return ErrInvalidTransition
	}
	next := uint32(1)
	if c.activeVersion != nil {
		next = c.activeVersion.number + 1
	}
	version, err := NewVersion(next, rule, dailyBudgetFen, impressionCostFen, frequencyLimit, now)
	if err != nil {
		return err
	}
	c.activeVersion = &version
	c.status = StatusActive
	c.revision++
	c.events = append(c.events, CampaignPublished{CampaignID: c.id, Version: next, At: now.UTC()})
	return nil
}

func (c *Campaign) UpdateDraft(name Name, slotID SlotID, period DeliveryPeriod) error {
	if c.status != StatusDraft {
		return ErrInvalidTransition
	}
	c.name = name
	c.slotID = slotID
	c.period = period
	c.revision++
	return nil
}

func (c *Campaign) Pause(now time.Time) error {
	if c.status != StatusActive {
		return ErrInvalidTransition
	}
	c.status = StatusPaused
	c.revision++
	c.events = append(c.events, CampaignPaused{CampaignID: c.id, At: now.UTC()})
	return nil
}

func (c *Campaign) Resume(now time.Time) error {
	if c.status != StatusPaused || c.activeVersion == nil {
		return ErrInvalidTransition
	}
	c.status = StatusActive
	c.revision++
	c.events = append(c.events, CampaignResumed{CampaignID: c.id, At: now.UTC()})
	return nil
}

func (c *Campaign) PullEvents() []Event {
	events := append([]Event(nil), c.events...)
	c.events = nil
	return events
}

func (c *Campaign) Clone() *Campaign {
	return Rehydrate(c.id, c.name, c.slotID, c.period, c.status, c.activeVersion, c.revision)
}

func cloneVersion(version *Version) *Version {
	if version == nil {
		return nil
	}
	copy := *version
	copy.targeting = version.targeting.Clone()
	return &copy
}

func (c *Campaign) ID() string              { return c.id }
func (c *Campaign) Name() Name              { return c.name }
func (c *Campaign) SlotID() SlotID          { return c.slotID }
func (c *Campaign) Period() DeliveryPeriod  { return c.period }
func (c *Campaign) Status() Status          { return c.status }
func (c *Campaign) Revision() uint64        { return c.revision }
func (c *Campaign) ActiveVersion() *Version { return cloneVersion(c.activeVersion) }
