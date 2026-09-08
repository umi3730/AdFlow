package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zhanghaiyang/adflow/internal/decision/domain"
)

func (r *Runtime) DeleteProfile(_ context.Context, userID string) error {
	userID = strings.TrimSpace(userID)
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.profiles, userID)
	return nil
}

type reservation struct {
	key       string
	amount    int64
	expiresAt time.Time
}

type storedDecision struct {
	result    domain.Result
	expiresAt time.Time
}

type Runtime struct {
	mu                    sync.Mutex
	profiles              map[string]domain.Profile
	decisions             map[string]storedDecision
	frequencyCounts       map[string]uint32
	frequencyReservations map[string]reservation
	budgetReserved        map[string]int64
	budgetReservations    map[string]reservation
	requestClaims         map[string]requestClaim
	requestSweep          time.Time
	settlementReceipts    map[string]settlementReceipt
}

func NewRuntime() *Runtime {
	return &Runtime{
		profiles: make(map[string]domain.Profile), decisions: make(map[string]storedDecision),
		frequencyCounts: make(map[string]uint32), frequencyReservations: make(map[string]reservation),
		budgetReserved: make(map[string]int64), budgetReservations: make(map[string]reservation),
		requestClaims:      make(map[string]requestClaim),
		settlementReceipts: make(map[string]settlementReceipt),
	}
}

func (r *Runtime) PutProfile(_ context.Context, profile domain.Profile) error {
	profile.UserID = strings.TrimSpace(profile.UserID)
	if profile.UserID == "" {
		return domain.ErrInvalidRequest
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.profiles[profile.UserID] = domain.NewProfile(profile.UserID, profileTags(profile), profile.Fields)
	return nil
}

func (r *Runtime) FindProfile(_ context.Context, userID string) (domain.Profile, error) {
	userID = strings.TrimSpace(userID)
	r.mu.Lock()
	defer r.mu.Unlock()
	profile, exists := r.profiles[userID]
	if !exists {
		return domain.Profile{}, domain.ErrProfileNotFound
	}
	return domain.NewProfile(profile.UserID, profileTags(profile), profile.Fields), nil
}

func (r *Runtime) ListProfiles(_ context.Context, filter domain.ProfileFilter) (domain.ProfilePage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]domain.Profile, 0)
	for _, profile := range r.profiles {
		if filter.Query != "" && !strings.Contains(profile.UserID, filter.Query) {
			continue
		}
		if filter.Tag != "" {
			if _, ok := profile.Tags[filter.Tag]; !ok {
				continue
			}
		}
		if filter.Device != "" && profile.Fields["device"] != filter.Device {
			continue
		}
		items = append(items, domain.NewProfile(profile.UserID, profileTags(profile), profile.Fields))
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UserID < items[j].UserID })
	total := len(items)
	limit, offset := filter.Limit, max(filter.Offset, 0)
	if limit < 1 || limit > 100 {
		limit = 20
	}
	start := min(offset, total)
	end := min(start+limit, total)
	return domain.ProfilePage{Items: items[start:end], Total: total}, nil
}

func (r *Runtime) FindDecision(_ context.Context, requestID string) (domain.Result, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	r.cleanupExpired(now)
	stored, exists := r.decisions[requestID]
	if !exists || (!stored.expiresAt.IsZero() && !now.Before(stored.expiresAt)) {
		delete(r.decisions, requestID)
		return domain.Result{}, false, nil
	}
	return stored.result, true, nil
}

func (r *Runtime) FindDecisions(_ context.Context, requestIDs []string) (map[string]domain.Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	r.cleanupExpired(now)
	result := make(map[string]domain.Result, len(requestIDs))
	for _, requestID := range requestIDs {
		stored, exists := r.decisions[requestID]
		if exists && (stored.expiresAt.IsZero() || now.Before(stored.expiresAt)) {
			result[requestID] = stored.result
		}
	}
	return result, nil
}

func (r *Runtime) SaveDecision(_ context.Context, result domain.Result) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	expiresAt := time.Now().UTC().Add(5 * time.Minute)
	if existing, exists := r.decisions[result.RequestID]; exists && existing.result != result {
		return domain.ErrInvalidRequest
	}
	r.decisions[result.RequestID] = storedDecision{result: result, expiresAt: expiresAt}
	return nil
}

func (r *Runtime) ReserveFrequency(_ context.Context, userID, campaignID, requestID string, limit uint32, now time.Time, ttl time.Duration) (string, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cleanupExpired(now)
	if _, exists := r.frequencyReservations[requestID]; exists {
		return requestID, true, nil
	}
	key := fmt.Sprintf("%s:%s:%s", now.UTC().Format("2006-01-02"), campaignID, userID)
	if r.frequencyCounts[key] >= limit {
		return "", false, nil
	}
	r.frequencyCounts[key]++
	r.frequencyReservations[requestID] = reservation{key: key, expiresAt: now.Add(ttl)}
	return requestID, true, nil
}

func (r *Runtime) ReleaseFrequency(_ context.Context, token string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.releaseFrequency(token)
	return nil
}

func (r *Runtime) ConfirmFrequency(_ context.Context, token string, _ time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.frequencyReservations, token)
	return nil
}

func (r *Runtime) ReserveBudget(_ context.Context, campaignID string, dailyBudgetFen, costFen int64, requestID string, now time.Time, ttl time.Duration) (string, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cleanupExpired(now)
	if _, exists := r.budgetReservations[requestID]; exists {
		return requestID, true, nil
	}
	key := fmt.Sprintf("%s:%s", now.UTC().Format("2006-01-02"), campaignID)
	if costFen <= 0 || dailyBudgetFen < costFen || r.budgetReserved[key]+costFen > dailyBudgetFen {
		return "", false, nil
	}
	r.budgetReserved[key] += costFen
	r.budgetReservations[requestID] = reservation{key: key, amount: costFen, expiresAt: now.Add(ttl)}
	return requestID, true, nil
}

func (r *Runtime) ReleaseBudget(_ context.Context, token string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.releaseBudget(token)
	return nil
}

func (r *Runtime) ConfirmBudget(_ context.Context, token string, _ time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.budgetReservations, token)
	return nil
}

func (r *Runtime) cleanupExpired(now time.Time) {
	for token, item := range r.frequencyReservations {
		if !now.Before(item.expiresAt) {
			r.releaseFrequency(token)
		}
	}
	for token, item := range r.budgetReservations {
		if !now.Before(item.expiresAt) {
			r.releaseBudget(token)
		}
	}
}

func (r *Runtime) releaseFrequency(token string) {
	item, exists := r.frequencyReservations[token]
	if !exists {
		return
	}
	if r.frequencyCounts[item.key] <= 1 {
		delete(r.frequencyCounts, item.key)
	} else {
		r.frequencyCounts[item.key]--
	}
	delete(r.frequencyReservations, token)
}

func (r *Runtime) releaseBudget(token string) {
	item, exists := r.budgetReservations[token]
	if !exists {
		return
	}
	if r.budgetReserved[item.key] <= item.amount {
		delete(r.budgetReserved, item.key)
	} else {
		r.budgetReserved[item.key] -= item.amount
	}
	delete(r.budgetReservations, token)
}

func profileTags(profile domain.Profile) []string {
	tags := make([]string, 0, len(profile.Tags))
	for tag := range profile.Tags {
		tags = append(tags, tag)
	}
	return tags
}
