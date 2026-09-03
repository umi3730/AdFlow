package domain

import "time"

type Event interface {
	EventName() string
}

type CampaignPublished struct {
	CampaignID string
	Version    uint32
	At         time.Time
}

func (CampaignPublished) EventName() string { return "campaign.published" }

type CampaignPaused struct {
	CampaignID string
	At         time.Time
}

func (CampaignPaused) EventName() string { return "campaign.paused" }

type CampaignResumed struct {
	CampaignID string
	At         time.Time
}

func (CampaignResumed) EventName() string { return "campaign.resumed" }
