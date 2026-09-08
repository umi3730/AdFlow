package application

import (
	"context"
	"errors"
	"github.com/umi3730/adflow/internal/decision/adapter/memory"
	"github.com/umi3730/adflow/internal/decision/domain"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type controlledExecutionStore struct {
	*memory.Runtime
	entered   chan struct{}
	proceed   chan struct{}
	busy      chan struct{}
	enterOnce sync.Once
	busyOnce  sync.Once
	commits   atomic.Int32
	ackLost   bool
	uncertain bool
}

func (s *controlledExecutionStore) AcquireDecision(ctx context.Context, r domain.Request, owner string, ttl time.Duration) error {
	err := s.Runtime.AcquireDecision(ctx, r, owner, ttl)
	if errors.Is(err, domain.ErrDecisionInProgress) && s.busy != nil {
		s.busyOnce.Do(func() { close(s.busy) })
	}
	return err
}
func (s *controlledExecutionStore) CommitDecision(ctx context.Context, r domain.Result, owner string) error {
	s.commits.Add(1)
	if s.entered != nil {
		s.enterOnce.Do(func() { close(s.entered) })
	}
	if s.proceed != nil {
		select {
		case <-s.proceed:
		case <-ctx.Done():
			return errors.Join(domain.ErrDecisionNotCommitted, ctx.Err())
		}
	}
	if s.uncertain {
		return errors.New("unknown commit outcome")
	}
	if err := s.Runtime.CommitDecision(ctx, r, owner); err != nil {
		return err
	}
	if s.ackLost {
		return errors.New("commit acknowledgement lost")
	}
	return nil
}

func executionFixture(t *testing.T) (*memory.Runtime, domain.Candidate, domain.Request) {
	t.Helper()
	runtime := memory.NewRuntime()
	if err := runtime.PutProfile(t.Context(), domain.NewProfile("user", []string{"auction"}, nil)); err != nil {
		t.Fatal(err)
	}
	c := bidCandidate("winner", "studio", 5)
	c.DailyBudgetFen = 5
	c.FrequencyLimit = 1
	return runtime, c, domain.Request{RequestID: "same-request", UserID: "user", SlotID: "slot"}
}

func TestRequestBindingRejectsChangedUserSlotAndInlineProfile(t *testing.T) {
	runtime, c, request := executionFixture(t)
	engine := NewService(candidateProvider{[]domain.Candidate{c}}, runtime, runtime, runtime, runtime)
	request.ProfileDigest = "profile-a"
	first, err := engine.Decide(t.Context(), request)
	if err != nil || !first.Matched {
		t.Fatal(err)
	}
	for _, changed := range []domain.Request{{RequestID: request.RequestID, UserID: "other", SlotID: "slot", ProfileDigest: "profile-a"}, {RequestID: request.RequestID, UserID: "user", SlotID: "other", ProfileDigest: "profile-a"}, {RequestID: request.RequestID, UserID: "user", SlotID: "slot", ProfileDigest: "profile-b"}} {
		if _, err := engine.Decide(t.Context(), changed); !errors.Is(err, domain.ErrRequestConflict) {
			t.Fatalf("changed request accepted: %+v %v", changed, err)
		}
	}
	again, err := engine.Decide(t.Context(), request)
	if err != nil || again != first {
		t.Fatalf("retry=%+v err=%v", again, err)
	}
	if first.ExpiresAt.Nanosecond()%int(time.Millisecond) != 0 {
		t.Fatal("expiry does not match DB precision")
	}
}

func TestConcurrentSameRequestAcrossServicesReusesResultAndKeepsReservations(t *testing.T) {
	runtime, c, request := executionFixture(t)
	store := &controlledExecutionStore{Runtime: runtime, entered: make(chan struct{}), proceed: make(chan struct{}), busy: make(chan struct{})}
	firstService := NewService(candidateProvider{[]domain.Candidate{c}}, runtime, runtime, runtime, store)
	secondService := NewService(candidateProvider{[]domain.Candidate{c}}, runtime, runtime, runtime, store)
	type response struct {
		result domain.Result
		err    error
	}
	responses := make(chan response, 2)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	go func() { result, err := firstService.Decide(ctx, request); responses <- response{result, err} }()
	select {
	case <-store.entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	go func() {
		copy := request
		copy.Now = time.Now().Add(time.Millisecond)
		result, err := secondService.Decide(ctx, copy)
		responses <- response{result, err}
	}()
	select {
	case <-store.busy:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	close(store.proceed)
	a, b := <-responses, <-responses
	if a.err != nil || b.err != nil || a.result != b.result || !a.result.Matched || store.commits.Load() != 1 {
		t.Fatalf("first=%+v second=%+v commits=%d", a, b, store.commits.Load())
	}
	if a.result.ReservationToken == request.RequestID {
		t.Fatal("execution still uses shared request token")
	}
	_, allowed, _ := runtime.ReserveBudget(t.Context(), c.CampaignID, 5, 5, "budget-probe", time.Now(), time.Second)
	if allowed {
		t.Fatal("winner budget was released")
	}
	_, allowed, _ = runtime.ReserveFrequency(t.Context(), "user", c.CampaignID, "frequency-probe", 1, time.Now(), time.Second)
	if allowed {
		t.Fatal("winner frequency was released")
	}
}

func TestCanceledWaiterDoesNotCancelOrReleaseTheOwner(t *testing.T) {
	runtime, c, request := executionFixture(t)
	store := &controlledExecutionStore{Runtime: runtime, entered: make(chan struct{}), proceed: make(chan struct{}), busy: make(chan struct{})}
	engine := NewService(candidateProvider{[]domain.Candidate{c}}, runtime, runtime, runtime, store)
	ownerDone := make(chan error, 1)
	go func() { _, err := engine.Decide(t.Context(), request); ownerDone <- err }()
	<-store.entered
	ctx, cancel := context.WithCancel(t.Context())
	waiterDone := make(chan error, 1)
	go func() { _, err := engine.Decide(ctx, request); waiterDone <- err }()
	<-store.busy
	cancel()
	if err := <-waiterDone; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	close(store.proceed)
	if err := <-ownerDone; err != nil {
		t.Fatal(err)
	}
	_, allowed, _ := runtime.ReserveBudget(t.Context(), c.CampaignID, 5, 5, "probe", time.Now(), time.Second)
	if allowed {
		t.Fatal("waiter released owner reservation")
	}
}

func TestLostCommitAcknowledgementReturnsSavedResultWithoutReleasingBudget(t *testing.T) {
	runtime, c, request := executionFixture(t)
	store := &controlledExecutionStore{Runtime: runtime, ackLost: true}
	engine := NewService(candidateProvider{[]domain.Candidate{c}}, runtime, runtime, runtime, store)
	result, err := engine.Decide(t.Context(), request)
	if err != nil || !result.Matched {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	_, allowed, _ := runtime.ReserveBudget(t.Context(), c.CampaignID, 5, 5, "probe", time.Now(), time.Second)
	if allowed {
		t.Fatal("uncertain commit released saved winner")
	}
}

func TestUnknownCommitKeepsOnlyOwnReservationUntilTTL(t *testing.T) {
	runtime, c, request := executionFixture(t)
	store := &controlledExecutionStore{Runtime: runtime, uncertain: true}
	engine := NewService(candidateProvider{[]domain.Candidate{c}}, runtime, runtime, runtime, store)
	now := time.Now().UTC()
	request.Now = now
	if _, err := engine.Decide(t.Context(), request); err == nil {
		t.Fatal("expected uncertain error")
	}
	_, allowed, _ := runtime.ReserveBudget(t.Context(), c.CampaignID, 5, 5, "probe-before", now, time.Second)
	if allowed {
		t.Fatal("released before commit was resolved")
	}
	_, allowed, _ = runtime.ReserveBudget(t.Context(), c.CampaignID, 5, 5, "probe-after", now.Add(31*time.Second), time.Second)
	if !allowed {
		t.Fatal("unknown reservation did not expire")
	}
}

func TestReservationIdentifiersSeparateOwnersAndCandidates(t *testing.T) {
	a := executionReservationID("owner-a", "campaign-a")
	if a == executionReservationID("owner-b", "campaign-a") || a == executionReservationID("owner-a", "campaign-b") || len(a) > 128 {
		t.Fatal("reservation scope collision")
	}
}
