//go:build integration

package integration

import (
	"context"
	"errors"
	"github.com/redis/go-redis/v9"
	memory "github.com/umi3730/adflow/internal/decision/adapter/memory"
	profilemysql "github.com/umi3730/adflow/internal/decision/adapter/mysql"
	profilecache "github.com/umi3730/adflow/internal/decision/adapter/profilecache"
	reservationredis "github.com/umi3730/adflow/internal/decision/adapter/redis"
	decisionapp "github.com/umi3730/adflow/internal/decision/application"
	"github.com/umi3730/adflow/internal/decision/domain"
	"strings"
	"testing"
	"time"
)

type identityCandidates struct{}

func (identityCandidates) ActiveCandidates(context.Context, string, time.Time) ([]domain.Candidate, error) {
	return []domain.Candidate{{CampaignID: "identity-campaign", CreativeIDs: []string{"creative"}, FrequencyLimit: 1, DailyBudgetFen: 100, ImpressionCostFen: 1}}, nil
}

func TestOpaqueProfileIDsAgreeAcrossMySQLCacheAndFrequency(t *testing.T) {
	db := integrationMySQL(t)
	source := profilemysql.NewProfileStore(db)
	client := redis.NewClient(&redis.Options{Addr: integrationEnv(t, "ADFLOW_IT_REDIS_ADDR"), Protocol: 2, DisableIdentity: true})
	t.Cleanup(func() { _ = client.Close() })
	prefix := "adflow:it:identity:" + newID(t)
	t.Cleanup(func() {
		if err := deletePrefix(context.Background(), client, prefix); err != nil {
			t.Error(err)
		}
	})
	id := "user-" + newID(t)
	upper := strings.ToUpper(id)
	t.Cleanup(func() {
		_ = source.DeleteProfile(context.Background(), id)
		_ = source.DeleteProfile(context.Background(), upper)
	})
	cache, err := profilecache.New(source, client, prefix, time.Minute, time.Second, time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.PutProfile(t.Context(), domain.NewProfile(id, nil, map[string]string{"score": "10"})); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.FindProfile(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.FindProfile(t.Context(), upper); !errors.Is(err, domain.ErrProfileNotFound) {
		t.Fatalf("uppercase alias resolved without a distinct profile: %v", err)
	}
	reservations := reservationredis.NewReservations(client, prefix+":reservations")
	service := decisionapp.NewService(identityCandidates{}, cache, reservations, reservations, memory.NewRuntime())
	first, err := service.Decide(t.Context(), domain.Request{RequestID: "first", UserID: id, SlotID: "slot"})
	if err != nil || !first.Matched {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	alias, err := service.Decide(t.Context(), domain.Request{RequestID: "alias", UserID: upper, SlotID: "slot"})
	if err != nil || alias.Matched {
		t.Fatalf("alias bypassed identity: %+v %v", alias, err)
	}
	capped, err := service.Decide(t.Context(), domain.Request{RequestID: "capped", UserID: id, SlotID: "slot"})
	if err != nil || capped.Reason != domain.ReasonFrequencyCapped {
		t.Fatalf("cap=%+v %v", capped, err)
	}
	// Explicitly creating the other ID creates a different user, not an update.
	if err := cache.PutProfile(t.Context(), domain.NewProfile(upper, nil, map[string]string{"score": "99"})); err != nil {
		t.Fatal(err)
	}
	stored, err := source.FindProfile(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	cached, err := cache.FindProfile(t.Context(), id)
	if err != nil || cached.Fields["score"] != "10" || stored.Fields["score"] != "10" {
		t.Fatalf("different ID changed lower profile: %+v %+v %v", stored, cached, err)
	}
	other, err := cache.FindProfile(t.Context(), upper)
	if err != nil || other.Fields["score"] != "99" {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM user_profiles WHERE user_id IN (?, ?)", id, upper).Scan(&count); err != nil || count != 2 {
		t.Fatalf("rows=%d err=%v", count, err)
	}
	if err := cache.DeleteProfile(t.Context(), upper); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.FindProfile(t.Context(), upper); !errors.Is(err, domain.ErrProfileNotFound) {
		t.Fatal(err)
	}
	if _, err := cache.FindProfile(t.Context(), id); err != nil {
		t.Fatal("other profile deleted", err)
	}
}

func TestProfileIDsPreserveAccentsAndNormalizeSurroundingWhitespace(t *testing.T) {
	db := integrationMySQL(t)
	source := profilemysql.NewProfileStore(db)
	id := "resume-" + newID(t)
	accent := strings.Replace(id, "resume", "résumé", 1)
	t.Cleanup(func() {
		_ = source.DeleteProfile(context.Background(), id)
		_ = source.DeleteProfile(context.Background(), accent)
	})
	if err := source.PutProfile(t.Context(), domain.NewProfile(" "+id+" ", nil, nil)); err != nil {
		t.Fatal(err)
	}
	if _, err := source.FindProfile(t.Context(), accent); !errors.Is(err, domain.ErrProfileNotFound) {
		t.Fatalf("accent alias found: %v", err)
	}
	got, err := source.FindProfile(t.Context(), " "+id+" ")
	if err != nil || got.UserID != id {
		t.Fatalf("id=%q err=%v", got.UserID, err)
	}
	var count int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM user_profiles WHERE user_id = ?", id+" ").Scan(&count); err != nil || count != 0 {
		t.Fatalf("database still pads opaque IDs: count=%d err=%v", count, err)
	}
}

func TestRedisFixtureCleanupFinishesBeforeClientClose(t *testing.T) {
	address := integrationEnv(t, "ADFLOW_IT_REDIS_ADDR")
	prefix := "adflow:it:cleanup:" + newID(t)
	t.Run("owned-fixture", func(t *testing.T) {
		client := redis.NewClient(&redis.Options{Addr: address, Protocol: 2, DisableIdentity: true})
		t.Cleanup(func() {
			if err := client.Close(); err != nil {
				t.Error(err)
			}
		})
		t.Cleanup(func() {
			if err := deletePrefix(context.Background(), client, prefix); err != nil {
				t.Error(err)
			}
		})
		if err := client.Set(t.Context(), prefix+":key", "value", time.Minute).Err(); err != nil {
			t.Fatal(err)
		}
	})
	verifier := redis.NewClient(&redis.Options{Addr: address, Protocol: 2, DisableIdentity: true})
	defer verifier.Close()
	count, err := verifier.Exists(t.Context(), prefix+":key").Result()
	if err != nil || count != 0 {
		t.Fatalf("fixture keys remain=%d err=%v", count, err)
	}
	if err := verifier.Close(); err != nil {
		t.Fatal(err)
	}
	if err := deletePrefix(t.Context(), verifier, prefix); err == nil {
		t.Fatal("cleanup failure silently ignored")
	}
}
