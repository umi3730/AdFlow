package domain

type Status string

const (
	StatusDraft   Status = "DRAFT"
	StatusActive  Status = "ACTIVE"
	StatusPaused  Status = "PAUSED"
	StatusEnded   Status = "ENDED"
	StatusDeleted Status = "DELETED"
)
