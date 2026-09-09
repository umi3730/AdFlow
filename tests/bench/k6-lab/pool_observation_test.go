package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/umi3730/adflow/internal/platform/profiling"
)

func TestPoolObservationUsesDeltasAndValidatesInstance(t *testing.T) {
	info := profiling.InstanceInfo{APIAddress: "127.0.0.1:18080", PID: 42, StartedAt: time.Now().UTC(), Pool: sql.DBStats{MaxOpenConnections: 30, WaitCount: 5, WaitDuration: time.Second}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(info) }))
	defer server.Close()
	h := chainCapacity{base: "http://127.0.0.1:18080", report: map[string]any{"profilingBase": server.URL, "poolMaxOpen": 30}}
	s, err := h.beginPoolStage("test", "baseline")
	if err != nil {
		t.Fatal(err)
	}
	info.Pool.InUse = 12
	info.Pool.OpenConnections = 14
	if err = s.sample(); err != nil {
		t.Fatal(err)
	}
	result, err := s.finish()
	if err != nil {
		t.Fatal(err)
	}
	if result["waitCountDelta"] != int64(0) || result["waitDurationSeconds"] != float64(0) || result["sampledPeakInUse"] != 12 {
		t.Fatalf("incorrect delta/peak: %v", result)
	}
	info.PID++
	if err = s.sample(); err == nil {
		t.Fatal("process change was not detected")
	}
	info.APIAddress = "127.0.0.1:18081"
	if _, err = h.beginPoolStage("bad", "baseline"); err == nil {
		t.Fatal("wrong API was not rejected")
	}
}
