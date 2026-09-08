package mysql

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	decisiondomain "github.com/umi3730/adflow/internal/decision/domain"
	"github.com/umi3730/adflow/internal/event/domain"
	"testing"
	"time"
)

func TestSettlementIngressTransactionGatesImpressionAndClick(t *testing.T) {
	for _, state := range []string{"PENDING", "SETTLED", "RECONCILE"} {
		t.Run(state, func(t *testing.T) {
			for _, eventType := range []domain.Type{domain.Impression, domain.Click} {
				t.Run(string(eventType), func(t *testing.T) {
					db, mock := mockDB(t)
					event := sampleEvent()
					event.Type = eventType
					mock.ExpectBegin()
					if eventType == domain.Impression {
						mock.ExpectExec("INSERT IGNORE INTO event_settlements").WillReturnResult(sqlmock.NewResult(0, 1))
					}
					mock.ExpectQuery("SELECT event_id, status.*FOR UPDATE").WithArgs(event.RequestID).
						WillReturnRows(sqlmock.NewRows([]string{"event_id", "status"}).AddRow(event.EventID, state))
					mock.ExpectExec("INSERT IGNORE INTO event_receipts").WillReturnResult(sqlmock.NewResult(0, 1))
					outboxStatus := "SETTLING"
					if state == "SETTLED" {
						outboxStatus = "PENDING"
					}
					if state == "RECONCILE" {
						outboxStatus = "RECONCILE"
					}
					mock.ExpectExec("INSERT INTO event_outbox").WithArgs(event.EventID, event.RequestID, sqlmock.AnyArg(), outboxStatus, event.OccurredAt).WillReturnResult(sqlmock.NewResult(0, 1))
					mock.ExpectCommit()
					if created, err := NewOutbox(db, "worker").EnqueueForSettlement(t.Context(), event, decisiondomain.Result{}, event.OccurredAt); err != nil || !created {
						t.Fatalf("created=%v err=%v", created, err)
					}
					if err := mock.ExpectationsWereMet(); err != nil {
						t.Fatal(err)
					}
				})
			}
		})
	}
}

func TestSettlementIngressRejectsSecondImpressionAndClickWithoutImpression(t *testing.T) {
	for _, eventType := range []domain.Type{domain.Impression, domain.Click} {
		t.Run(string(eventType), func(t *testing.T) {
			db, mock := mockDB(t)
			event := sampleEvent()
			event.Type = eventType
			mock.ExpectBegin()
			if eventType == domain.Impression {
				mock.ExpectExec("INSERT IGNORE INTO event_settlements").WillReturnResult(sqlmock.NewResult(0, 0))
			}
			query := mock.ExpectQuery("SELECT event_id, status.*FOR UPDATE")
			want := domain.ErrImpressionRequired
			if eventType == domain.Impression {
				query.WillReturnRows(sqlmock.NewRows([]string{"event_id", "status"}).AddRow("first-impression", "PENDING"))
				want = domain.ErrEventConflict
			} else {
				query.WillReturnError(sql.ErrNoRows)
			}
			mock.ExpectRollback()
			if _, err := NewOutbox(db, "worker").EnqueueForSettlement(t.Context(), event, decisiondomain.Result{}, event.OccurredAt); !errors.Is(err, want) {
				t.Fatal(err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSettlementIngressRollsBackIfOutboxWriteFails(t *testing.T) {
	db, mock := mockDB(t)
	event := sampleEvent()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT IGNORE INTO event_settlements").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT event_id, status.*FOR UPDATE").WillReturnRows(sqlmock.NewRows([]string{"event_id", "status"}).AddRow(event.EventID, "PENDING"))
	mock.ExpectExec("INSERT IGNORE INTO event_receipts").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO event_outbox").WillReturnError(errors.New("disk full"))
	mock.ExpectRollback()
	if _, err := NewOutbox(db, "worker").EnqueueForSettlement(t.Context(), event, decisiondomain.Result{}, event.OccurredAt); err == nil {
		t.Fatal("expected rollback")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSettlementCompletionFencesExpiredOrReplacedWorker(t *testing.T) {
	for _, affected := range []int64{0, 1} {
		t.Run(string(rune('0'+affected)), func(t *testing.T) {
			db, mock := mockDB(t)
			entry := domain.SettlementEntry{Settlement: decisiondomain.Settlement{EventID: "event", Decision: decisiondomain.Result{RequestID: "request"}}, Owner: "old-owner"}
			mock.ExpectBegin()
			mock.ExpectExec("UPDATE event_settlements.*locked_until > UTC_TIMESTAMP").WithArgs("request", "event", "old-owner").WillReturnResult(sqlmock.NewResult(0, affected))
			if affected == 0 {
				mock.ExpectRollback()
			} else {
				mock.ExpectExec("UPDATE event_outbox.*status = 'SETTLING'").WithArgs("request").WillReturnResult(sqlmock.NewResult(0, 2))
				mock.ExpectCommit()
			}
			err := NewOutbox(db, "worker").CompleteSettlement(t.Context(), entry)
			if affected == 0 && !errors.Is(err, domain.ErrSettlementLeaseLost) || affected == 1 && err != nil {
				t.Fatal(err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSettlementClaimUsesDatabaseTimeAndUniqueOwner(t *testing.T) {
	db, mock := mockDB(t)
	snapshot, _ := json.Marshal(decisiondomain.Result{RequestID: "request"})
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT event_id, decision_snapshot, attempts.*FOR UPDATE SKIP LOCKED").WithArgs(10).WillReturnRows(sqlmock.NewRows([]string{"event_id", "decision_snapshot", "attempts"}).AddRow("event", snapshot, 2))
	mock.ExpectExec("UPDATE event_settlements.*WHERE event_id IN").WithArgs(sqlmock.AnyArg(), int64(30000000), "event").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	entries, err := NewOutbox(db, "worker").ClaimSettlements(t.Context(), 10, 30*time.Second)
	if err != nil || len(entries) != 1 || len(entries[0].Owner) != 32 || entries[0].Attempts != 2 {
		t.Fatalf("%+v %v", entries, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestConsumerRequiresProofAndExactAcceptedPayload(t *testing.T) {
	for _, mode := range []string{"pending", "changed", "settled"} {
		t.Run(mode, func(t *testing.T) {
			db, mock := mockDB(t)
			event := sampleEvent()
			payload, _ := json.Marshal(event)
			rows := sqlmock.NewRows([]string{"payload"})
			if mode != "pending" {
				rows.AddRow(payload)
			}
			mock.ExpectQuery("SELECT o.payload.*s.status = 'SETTLED'").WillReturnRows(rows)
			if mode == "changed" {
				event.ValueFen++
			}
			err := NewOutbox(db, "worker").VerifySettledEvents(t.Context(), []domain.Event{event})
			if mode == "pending" && !errors.Is(err, domain.ErrSettlementPending) || mode == "changed" && !errors.Is(err, domain.ErrEventConflict) || mode == "settled" && err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSettlementFailureCannotOverwriteNewOwner(t *testing.T) {
	db, mock := mockDB(t)
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE event_settlements.*locked_until > UTC_TIMESTAMP").
		WithArgs("RECONCILE", int64(1000000), "missing reservation", "request", "event", "old-owner").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	entry := domain.SettlementEntry{Settlement: decisiondomain.Settlement{EventID: "event", Decision: decisiondomain.Result{RequestID: "request"}}, Owner: "old-owner"}
	if err := NewOutbox(db, "worker").FailSettlement(t.Context(), entry, "missing reservation", time.Second, true); !errors.Is(err, domain.ErrSettlementLeaseLost) {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPublicationReceiptFailureRollsBackDependentRelease(t *testing.T) {
	db, mock := mockDB(t)
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE event_outbox").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE event_receipts").WillReturnError(errors.New("connection lost"))
	mock.ExpectRollback()
	if err := NewOutbox(db, "worker").MarkPublished(t.Context(), "event", time.Now()); err == nil {
		t.Fatal("expected rollback")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSettlementDuplicateRaceKeepsOriginalBinding(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "same", true: "changed"}[changed], func(t *testing.T) {
			db, mock := mockDB(t)
			event := sampleEvent()
			payload, _ := json.Marshal(event)
			mock.ExpectBegin()
			mock.ExpectExec("INSERT IGNORE INTO event_settlements").WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectQuery("SELECT event_id, status.*FOR UPDATE").WillReturnRows(sqlmock.NewRows([]string{"event_id", "status"}).AddRow(event.EventID, "PENDING"))
			mock.ExpectExec("INSERT IGNORE INTO event_receipts").WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectQuery("SELECT payload.*FOR UPDATE").WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(payload))
			mock.ExpectRollback()
			if changed {
				event.ValueFen++
			}
			created, err := NewOutbox(db, "worker").EnqueueForSettlement(t.Context(), event, decisiondomain.Result{}, event.OccurredAt)
			if created || changed && !errors.Is(err, domain.ErrEventConflict) || !changed && err != nil {
				t.Fatalf("created=%v err=%v", created, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
