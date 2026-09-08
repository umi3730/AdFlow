package mysql

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/umi3730/adflow/internal/decision/domain"
)

func TestDecisionStoreRoundTripMapping(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	expires := time.Date(2026, 9, 3, 1, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"request_id", "user_id", "slot_id", "matched", "campaign_id", "creative_id", "reservation_token", "expires_at", "reason", "pricing", "request_fingerprint"}).
		AddRow("request-1", "user-1", "slot-1", true, "campaign000000000000000000000001", "creative000000000000000000000001", "request-1", expires, "matched", nil, nil)
	mock.ExpectQuery("SELECT request_id").WithArgs("request-1").WillReturnRows(rows)
	result, found, err := store.FindDecision(context.Background(), "request-1")
	if err != nil || !found || !result.Matched || result.Reason != domain.ReasonMatched || !result.ExpiresAt.Equal(expires) {
		t.Fatalf("result=%+v found=%v err=%v", result, found, err)
	}
}

func TestDecisionStorePersistsPricingAndChecksItForIdempotency(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	expires := time.Date(2026, 9, 7, 1, 0, 0, 0, time.UTC)
	pricing := domain.Pricing{Mode: "first_price", AdvertiserID: "a", AdvertiserName: "甲广告主", BidFen: 5, PriceFen: 5, Version: 2, Advertisers: 3, Rank: 1}
	payload, _ := json.Marshal(pricing)
	input := domain.Result{RequestID: "priced", UserID: "user", SlotID: "slot", Matched: true, CampaignID: "campaign", CreativeID: "creative", ReservationToken: "priced", ExpiresAt: expires, Reason: domain.ReasonMatched, Pricing: pricing}
	mock.ExpectExec("INSERT IGNORE INTO decisions").WithArgs("priced", "user", "slot", true, "campaign", "creative", "priced", expires, "matched", payload, nil).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := store.SaveDecision(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	rows := sqlmock.NewRows([]string{"request_id", "user_id", "slot_id", "matched", "campaign_id", "creative_id", "reservation_token", "expires_at", "reason", "pricing", "request_fingerprint"}).AddRow("priced", "user", "slot", true, "campaign", "creative", "priced", expires, "matched", payload, nil)
	mock.ExpectQuery("SELECT request_id").WithArgs("priced").WillReturnRows(rows)
	result, found, err := store.FindDecision(t.Context(), "priced")
	if err != nil || !found || !sameDecision(result, input) {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	input.Pricing.PriceFen = 6
	if sameDecision(result, input) {
		t.Fatal("changed settlement treated as identical")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDecisionStoreInsertsIdempotencyRecord(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	result := domain.Result{RequestID: "request-1", UserID: "user-1", SlotID: "slot-1", Reason: domain.ReasonNoCandidate}
	mock.ExpectExec("INSERT IGNORE INTO decisions").
		WithArgs(result.RequestID, result.UserID, result.SlotID, false, "", "", "", nil, string(result.Reason), nil, nil).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := store.SaveDecision(context.Background(), result); err != nil {
		t.Fatal(err)
	}
}

func TestDecisionStoreFindsBatchInOneQuery(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	expires := time.Date(2026, 9, 3, 1, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"request_id", "user_id", "slot_id", "matched", "campaign_id", "creative_id", "reservation_token", "expires_at", "reason", "pricing", "request_fingerprint"}).
		AddRow("request-1", "user-1", "slot-1", true, "campaign-1", "creative-1", "request-1", expires, "matched", nil, nil).
		AddRow("request-2", "user-2", "slot-1", true, "campaign-2", "creative-2", "request-2", expires, "matched", nil, nil)
	mock.ExpectQuery("FROM decisions WHERE request_id IN").WithArgs("request-1", "request-2").WillReturnRows(rows)

	result, err := store.FindDecisions(context.Background(), []string{"request-1", "request-2", "request-1"})
	if err != nil || len(result) != 2 || result["request-2"].CreativeID != "creative-2" {
		t.Fatalf("result=%v err=%v", result, err)
	}
}
