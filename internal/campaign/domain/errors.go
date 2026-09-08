package domain

import "errors"

var (
	ErrDeleteActive       = errors.New("pause the campaign or disable the creative before deleting")
	ErrCampaignNotFound   = errors.New("campaign not found")
	ErrInvalidName        = errors.New("campaign name must contain 2 to 128 characters")
	ErrInvalidSlotID      = errors.New("slot id must contain 2 to 64 characters")
	ErrInvalidPeriod      = errors.New("delivery end must be after delivery start")
	ErrInvalidTransition  = errors.New("campaign status transition is not allowed")
	ErrInvalidBudget      = errors.New("daily budget must be positive")
	ErrInvalidCost        = errors.New("impression cost must be positive")
	ErrBudgetBelowCost    = errors.New("daily budget must cover at least one impression")
	ErrInvalidFrequency   = errors.New("frequency limit must be between 1 and 100")
	ErrInvalidTargeting   = errors.New("targeting rule is invalid")
	ErrConcurrentMutation = errors.New("campaign was modified concurrently")
	ErrCreativeNotFound   = errors.New("creative not found")
	ErrInvalidCreative    = errors.New("creative title and URLs are invalid")
)
