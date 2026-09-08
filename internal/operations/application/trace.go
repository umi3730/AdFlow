package application

import (
	"context"
	"errors"
	decisiondomain "github.com/umi3730/adflow/internal/decision/domain"
	"github.com/umi3730/adflow/internal/event/domain"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrTraceNotFound = errors.New("request has no retained inspection records")
var ErrTraceUnavailable = errors.New("request inspection is unavailable")
var ErrInvalidTraceID = errors.New("request ID must contain 1 to 128 characters")

type TraceDecision struct {
	UserID     string                  `json:"userId"`
	SlotID     string                  `json:"slotId"`
	Matched    bool                    `json:"matched"`
	CampaignID string                  `json:"campaignId,omitempty"`
	CreativeID string                  `json:"creativeId,omitempty"`
	Reason     decisiondomain.Reason   `json:"reason"`
	Pricing    *decisiondomain.Pricing `json:"pricing,omitempty"`
	ExpiresAt  *time.Time              `json:"expiresAt,omitempty"`
}
type RequestTrace struct {
	RequestID      string                  `json:"requestId"`
	EventTransport string                  `json:"eventTransport"`
	ObservedAt     time.Time               `json:"observedAt"`
	Decision       *TraceDecision          `json:"decision,omitempty"`
	Execution      *domain.TraceExecution  `json:"execution,omitempty"`
	Settlement     *domain.TraceSettlement `json:"settlement,omitempty"`
	Events         []domain.TraceEvent     `json:"events"`
	Truncated      bool                    `json:"truncated"`
}
type TraceService struct {
	decisions decisiondomain.DecisionInspector
	records   domain.RequestStateReader
	mode      string
}

func NewTraceService(decisions decisiondomain.DecisionInspector, records domain.RequestStateReader, mode string) *TraceService {
	return &TraceService{decisions: decisions, records: records, mode: mode}
}
func (s *TraceService) Read(ctx context.Context, id string) (RequestTrace, error) {
	id = strings.TrimSpace(id)
	result := RequestTrace{RequestID: id, EventTransport: s.mode, Events: []domain.TraceEvent{}}
	if id == "" || !utf8.ValidString(id) || utf8.RuneCountInString(id) > 128 {
		return result, ErrInvalidTraceID
	}
	if s.records == nil || s.decisions == nil {
		return result, ErrTraceUnavailable
	}
	state, err := s.records.ReadRequestState(ctx, id)
	if err != nil {
		return result, err
	}
	saved := state.SavedDecision
	if saved == nil {
		d, found, err := s.decisions.PeekDecision(ctx, id)
		if err != nil {
			return result, err
		}
		if found && d.RequestID == id {
			saved = &d
		}
	}
	if saved == nil && !state.Known {
		return result, ErrTraceNotFound
	}
	// Exposure deadlines use the application clock; database time is retained
	// internally for evaluating database execution leases.
	result.ObservedAt = time.Now().UTC()
	result.Execution = state.Execution
	result.Settlement = state.Settlement
	result.Events = state.Events
	if result.Events == nil {
		result.Events = []domain.TraceEvent{}
	}
	result.Truncated = state.Truncated
	if saved != nil {
		result.Decision = &TraceDecision{UserID: saved.UserID, SlotID: saved.SlotID, Matched: saved.Matched, CampaignID: saved.CampaignID, CreativeID: saved.CreativeID, Reason: saved.Reason}
		if saved.Pricing.Mode != "" {
			pricing := saved.Pricing
			result.Decision.Pricing = &pricing
		}
		if !saved.ExpiresAt.IsZero() {
			expires := saved.ExpiresAt
			result.Decision.ExpiresAt = &expires
		}
	}
	return result, nil
}
