package memory

import (
	"errors"
	"github.com/umi3730/adflow/internal/decision/domain"
	"testing"
	"time"
)

func TestExecutionLeaseFencesOldOwnerAndKeepsParameterBinding(t *testing.T) {
	r := NewRuntime()
	request := domain.Request{RequestID: "id", UserID: "user", SlotID: "slot"}
	if err := r.AcquireDecision(t.Context(), request, "old", time.Minute); err != nil {
		t.Fatal(err)
	}
	changed := request
	changed.UserID = "other"
	if err := r.AcquireDecision(t.Context(), changed, "other", time.Minute); !errors.Is(err, domain.ErrRequestConflict) {
		t.Fatal(err)
	}
	if err := r.AcquireDecision(t.Context(), request, "new", time.Minute); !errors.Is(err, domain.ErrDecisionInProgress) {
		t.Fatal(err)
	}
	r.mu.Lock()
	claim := r.requestClaims["id"]
	claim.leaseUntil = time.Now().Add(-time.Second)
	r.requestClaims["id"] = claim
	r.mu.Unlock()
	if err := r.AcquireDecision(t.Context(), request, "new", time.Minute); err != nil {
		t.Fatal(err)
	}
	result := domain.Result{RequestID: "id", UserID: "user", SlotID: "slot", RequestFingerprint: domain.RequestFingerprint(request), Reason: domain.ReasonNoCandidate}
	if err := r.CommitDecision(t.Context(), result, "old"); !errors.Is(err, domain.ErrDecisionExecutionLost) || !errors.Is(err, domain.ErrDecisionNotCommitted) {
		t.Fatal(err)
	}
	_ = r.ReleaseDecision(t.Context(), "id", "old")
	if err := r.AcquireDecision(t.Context(), request, "third", time.Minute); !errors.Is(err, domain.ErrDecisionInProgress) {
		t.Fatal("old owner released new lease", err)
	}
	if err := r.CommitDecision(t.Context(), result, "new"); err != nil {
		t.Fatal(err)
	}
	found, exists, err := r.FindDecision(t.Context(), "id")
	if err != nil || !exists || found != result {
		t.Fatalf("stored=%+v err=%v", found, err)
	}
}

func TestMatchedDecisionRetentionIsSeparateFromReservationExpiry(t *testing.T) {
	r := NewRuntime()
	request := domain.Request{RequestID: "expired-reservation", UserID: "user", SlotID: "slot"}
	if err := r.AcquireDecision(t.Context(), request, "owner", time.Minute); err != nil {
		t.Fatal(err)
	}
	result := domain.Result{RequestID: request.RequestID, UserID: request.UserID, SlotID: request.SlotID, RequestFingerprint: domain.RequestFingerprint(request), Matched: true, ExpiresAt: time.Now().Add(-time.Second)}
	if err := r.CommitDecision(t.Context(), result, "owner"); err != nil {
		t.Fatal(err)
	}
	stored, found, err := r.FindDecision(t.Context(), request.RequestID)
	if err != nil || !found || stored != result {
		t.Fatalf("expired reservation lost idempotency: %v %v", found, err)
	}
}
