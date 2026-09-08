//go:build integration

package integration

import (
	"fmt"
	"strings"
	"testing"

	eventmysql "github.com/umi3730/adflow/internal/event/adapter/mysql"
)

func TestBackpressureCountsActiveStatesAndCapsWithMySQL(t *testing.T) {
	db := integrationMySQL(t)
	var schema string
	if err := db.QueryRow("SELECT DATABASE()").Scan(&schema); err != nil || !strings.HasPrefix(schema, "adflow_backpressure_it_") {
		t.Fatal("requires an isolated adflow_backpressure_it_ schema", err)
	}
	reader := eventmysql.NewOutbox(db, "backlog-test")
	before, err := reader.ReadDecisionBacklog(t.Context(), 100, 600)
	if err != nil || before.Settlements != 0 || before.Outbox != 0 {
		t.Fatal("test requires empty active queues", before, err)
	}
	prefix := "pressure-" + newID(t)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM event_settlements WHERE request_id LIKE ?", prefix+"%")
		_, _ = db.Exec("DELETE FROM event_outbox WHERE event_id LIKE ?", prefix+"%")
		_, _ = db.Exec("DELETE FROM event_receipts WHERE event_id LIKE ?", prefix+"%")
	})
	insertReceipt := func(id string) {
		t.Helper()
		if _, err := db.Exec("INSERT INTO event_receipts (event_id,request_id,campaign_id,creative_id,event_type,value_fen,occurred_at,status) VALUES (?,?,'test-campaign','test-creative','impression',0,UTC_TIMESTAMP(3),'ACCEPTED')", id, id); err != nil {
			t.Fatal(err)
		}
	}
	for _, state := range []string{"PENDING", "PROCESSING", "SETTLED", "RECONCILE"} {
		id := prefix + state
		if _, err := db.Exec("INSERT INTO event_settlements (request_id,event_id,decision_snapshot,accepted_at,status) VALUES (?,?,'{}',UTC_TIMESTAMP(3),?)", id, id, state); err != nil {
			t.Fatal(err)
		}
	}
	for _, state := range []string{"SETTLING", "PENDING", "PROCESSING", "PUBLISHED", "DEAD_LETTERED", "RECONCILE"} {
		id := prefix + state
		insertReceipt(id)
		if _, err := db.Exec("INSERT INTO event_outbox (event_id,aggregate_key,payload,status) VALUES (?,?,'{}',?)", id, id, state); err != nil {
			t.Fatal(err)
		}
	}
	// Terminal history must not count toward pressure, including old reconciliation.
	for i := 0; i < 100; i++ {
		id := fmt.Sprintf("%s-history-%d", prefix, i)
		insertReceipt(id)
		if _, err := db.Exec("INSERT INTO event_outbox (event_id,aggregate_key,payload,status) VALUES (?,?,'{}','RECONCILE')", id, id); err != nil {
			t.Fatal(err)
		}
	}
	got, err := reader.ReadDecisionBacklog(t.Context(), 100, 600)
	if err != nil || got.Settlements != 2 || got.Outbox != 3 {
		t.Fatalf("active counts=%+v %v", got, err)
	}
	got, err = reader.ReadDecisionBacklog(t.Context(), 1, 2)
	if err != nil || got.Settlements != 1 || got.Outbox != 2 {
		t.Fatalf("capped counts=%+v %v", got, err)
	}
	if _, err = db.Exec("UPDATE event_settlements SET status='SETTLED' WHERE request_id LIKE ?", prefix+"%"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("UPDATE event_outbox SET status='PUBLISHED' WHERE event_id LIKE ?", prefix+"%"); err != nil {
		t.Fatal(err)
	}
	got, err = reader.ReadDecisionBacklog(t.Context(), 100, 600)
	if err != nil || got.Settlements != 0 || got.Outbox != 0 {
		t.Fatalf("drained counts=%+v %v", got, err)
	}
}
