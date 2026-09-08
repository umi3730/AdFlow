package mysql

import (
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/zhanghaiyang/adflow/internal/decision/domain"
	"testing"
	"time"
)

func TestAcquireDecisionBindsParametersAndUsesDatabaseLeaseTime(t *testing.T) {
	for _, kind := range []string{"new", "conflict", "busy", "expired"} {
		t.Run(kind, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			store := NewStore(db)
			request := domain.Request{RequestID: "id", UserID: "user", SlotID: "slot"}
			fp := domain.RequestFingerprint(request)
			storedFP := fp
			var owner any
			active := false
			if kind == "conflict" {
				storedFP = "different"
			}
			if kind == "busy" || kind == "expired" {
				owner = "other"
			}
			if kind == "busy" {
				active = true
			}
			mock.ExpectBegin()
			mock.ExpectExec("INSERT INTO decision_requests").WithArgs("id", fp).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery("SELECT request_fingerprint.*FOR UPDATE").WithArgs("id").WillReturnRows(sqlmock.NewRows([]string{"fingerprint", "owner", "active"}).AddRow(storedFP, owner, active))
			if kind == "conflict" || kind == "busy" {
				mock.ExpectRollback()
			} else {
				mock.ExpectExec("UPDATE decision_requests SET owner_id = .*TIMESTAMPADD").WithArgs("mine", int64(60000000), "id").WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			}
			err = store.AcquireDecision(t.Context(), request, "mine", time.Minute)
			if kind == "conflict" && !errors.Is(err, domain.ErrRequestConflict) {
				t.Fatal(err)
			}
			if kind == "busy" && !errors.Is(err, domain.ErrDecisionInProgress) {
				t.Fatal(err)
			}
			if (kind == "new" || kind == "expired") && err != nil {
				t.Fatal(err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCommitDecisionFencesOwnershipAndDistinguishesUncertainCommit(t *testing.T) {
	for _, kind := range []string{"success", "stale", "uncertain"} {
		t.Run(kind, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			store := NewStore(db)
			result := domain.Result{RequestID: "id", UserID: "user", SlotID: "slot", Reason: domain.ReasonNoCandidate, RequestFingerprint: "fp"}
			owner := "mine"
			if kind == "stale" {
				owner = "new-owner"
			}
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT request_fingerprint.*FOR UPDATE").WithArgs("id").WillReturnRows(sqlmock.NewRows([]string{"fp", "owner", "active"}).AddRow("fp", owner, true))
			if kind == "stale" {
				mock.ExpectRollback()
			} else {
				mock.ExpectExec("INSERT IGNORE INTO decisions").WithArgs("id", "user", "slot", false, "", "", "", nil, "no_candidate", nil, "fp").WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec("UPDATE decision_requests SET owner_id = NULL").WithArgs("id", "mine").WillReturnResult(sqlmock.NewResult(0, 1))
				if kind == "uncertain" {
					mock.ExpectCommit().WillReturnError(errors.New("commit ack lost"))
				} else {
					mock.ExpectCommit()
				}
			}
			err = store.CommitDecision(t.Context(), result, "mine")
			if kind == "stale" && (!errors.Is(err, domain.ErrDecisionExecutionLost) || !errors.Is(err, domain.ErrDecisionNotCommitted)) {
				t.Fatal(err)
			}
			if kind == "uncertain" && (err == nil || errors.Is(err, domain.ErrDecisionNotCommitted)) {
				t.Fatalf("unsafe certainty: %v", err)
			}
			if kind == "success" && err != nil {
				t.Fatal(err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
