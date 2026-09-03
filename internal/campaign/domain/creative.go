package domain

import (
	"net/url"
	"strings"
)

type CreativeStatus string

const (
	CreativeActive   CreativeStatus = "ACTIVE"
	CreativeDisabled CreativeStatus = "DISABLED"
)

type Creative struct {
	id          string
	campaignID  string
	title       string
	description string
	imageURL    string
	landingURL  string
	status      CreativeStatus
	revision    uint64
}

func NewCreative(id, campaignID, title, description, imageURL, landingURL string) (*Creative, error) {
	title = strings.TrimSpace(title)
	if id == "" || campaignID == "" || len([]rune(title)) < 2 || len([]rune(title)) > 128 || !validHTTPURL(imageURL) || !validHTTPURL(landingURL) {
		return nil, ErrInvalidCreative
	}
	if len([]rune(description)) > 512 {
		return nil, ErrInvalidCreative
	}
	return &Creative{
		id: id, campaignID: campaignID, title: title, description: strings.TrimSpace(description),
		imageURL: imageURL, landingURL: landingURL, status: CreativeActive, revision: 1,
	}, nil
}

func RehydrateCreative(id, campaignID, title, description, imageURL, landingURL string, status CreativeStatus, revision uint64) *Creative {
	return &Creative{
		id: id, campaignID: campaignID, title: title, description: description,
		imageURL: imageURL, landingURL: landingURL, status: status, revision: revision,
	}
}

func (c *Creative) Disable() error {
	if c.status != CreativeActive {
		return ErrInvalidTransition
	}
	c.status = CreativeDisabled
	c.revision++
	return nil
}

func (c *Creative) Clone() *Creative {
	return RehydrateCreative(c.id, c.campaignID, c.title, c.description, c.imageURL, c.landingURL, c.status, c.revision)
}

func validHTTPURL(value string) bool {
	parsed, err := url.ParseRequestURI(value)
	return err == nil && parsed.Host != "" && (parsed.Scheme == "http" || parsed.Scheme == "https")
}

func (c *Creative) ID() string             { return c.id }
func (c *Creative) CampaignID() string     { return c.campaignID }
func (c *Creative) Title() string          { return c.title }
func (c *Creative) Description() string    { return c.description }
func (c *Creative) ImageURL() string       { return c.imageURL }
func (c *Creative) LandingURL() string     { return c.landingURL }
func (c *Creative) Status() CreativeStatus { return c.status }
func (c *Creative) Revision() uint64       { return c.revision }
