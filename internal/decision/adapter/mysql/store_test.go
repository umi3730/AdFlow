package mysql

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/zhanghaiyang/adflow/internal/decision/domain"
)

func TestDecisionStoreRoundTripMapping(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	expires := time.Date(2026, 9, 3, 1, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"request_id", "user_id", "slot_id", "matched", "campaign_id", "creative_id", "reservation_token", "expires_at", "reason"}).
		AddRow("request-1", "user-1", "slot-1", true, "campaign000000000000000000000001", "creative000000000000000000000001", "request-1", expires, "matched")
	mock.ExpectQuery("SELECT request_id").WithArgs("request-1").WillReturnRows(rows)
	result, found, err := store.FindDecision(context.Background(), "request-1")
	if err != nil || !found || !result.Matched || result.Reason != domain.ReasonMatched || !result.ExpiresAt.Equal(expires) {
		t.Fatalf("result=%+v found=%v err=%v", result, found, err)
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
		WithArgs(result.RequestID, result.UserID, result.SlotID, false, "", "", "", nil, string(result.Reason)).
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
	rows := sqlmock.NewRows([]string{"request_id", "user_id", "slot_id", "matched", "campaign_id", "creative_id", "reservation_token", "expires_at", "reason"}).
		AddRow("request-1", "user-1", "slot-1", true, "campaign-1", "creative-1", "request-1", expires, "matched").
		AddRow("request-2", "user-2", "slot-1", true, "campaign-2", "creative-2", "request-2", expires, "matched")
	mock.ExpectQuery("FROM decisions WHERE request_id IN").WithArgs("request-1", "request-2").WillReturnRows(rows)

	result, err := store.FindDecisions(context.Background(), []string{"request-1", "request-2", "request-1"})
	if err != nil || len(result) != 2 || result["request-2"].CreativeID != "creative-2" {
		t.Fatalf("result=%v err=%v", result, err)
	}
}
