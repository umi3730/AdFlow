package mysql

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/zhanghaiyang/adflow/internal/audit/domain"
	identity "github.com/zhanghaiyang/adflow/internal/identity/domain"
)

func TestAppend(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	entry := domain.Entry{ID: "audit-1", ActorID: "user-1", ActorName: "alice", ActorRole: identity.RoleAdmin,
		Action: "PUBLISH_CAMPAIGN", ResourceType: "campaign", ResourceID: "campaign-1", RequestID: "request-1",
		Outcome: domain.OutcomeSucceeded, Metadata: map[string]string{"method": "POST"}, CreatedAt: time.Now().UTC()}
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO audit_logs")).
		WithArgs(entry.ID, entry.ActorID, entry.ActorName, entry.ActorRole, entry.Action, entry.ResourceType,
			entry.ResourceID, entry.RequestID, entry.Outcome, []byte(`{"method":"POST"}`), entry.CreatedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := NewStore(db).Append(context.Background(), entry); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestList(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	rows := sqlmock.NewRows([]string{"id", "actor_id", "actor_name", "actor_role", "action", "resource_type", "resource_id", "request_id", "outcome", "metadata", "created_at"}).
		AddRow("audit-1", "user-1", "alice", "admin", "PUBLISH_CAMPAIGN", "campaign", "campaign-1", "request-1", "SUCCEEDED", []byte(`{"status":"OK"}`), now)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, actor_id, actor_name, actor_role, action, resource_type,")).
		WithArgs(50, 0).
		WillReturnRows(rows)
	entries, err := NewStore(db).List(context.Background(), domain.Filter{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ActorRole != identity.RoleAdmin || entries[0].Metadata["status"] != "OK" {
		t.Fatalf("unexpected entries: %+v", entries)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
