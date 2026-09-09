package profiling

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestProfilingIdentityIsolationAndShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	s, err := Start(ctx, "127.0.0.1:0", "127.0.0.1:18081", 0, func() sql.DBStats { return sql.DBStats{MaxOpenConnections: 60, WaitCount: 7} }, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Get("http://" + s.Address() + "/debug/instance")
	if err != nil {
		t.Fatal(err)
	}
	var info InstanceInfo
	if err = json.NewDecoder(response.Body).Decode(&info); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if info.APIAddress != "127.0.0.1:18081" || info.PID <= 0 || info.Pool.MaxOpenConnections != 60 || info.Pool.WaitCount != 7 {
		t.Fatalf("wrong instance: %+v", info)
	}
	index, err := http.Get("http://" + s.Address() + "/debug/pprof/")
	if err != nil {
		t.Fatal(err)
	}
	index.Body.Close()
	if index.StatusCode != 200 {
		t.Fatal(index.StatusCode)
	}
	cancel()
	select {
	case <-s.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("profiler did not stop")
	}
}

func TestProfilingDisabledAndAddressValidation(t *testing.T) {
	s, err := Start(t.Context(), "", ":18080", 0, nil, nil)
	if err != nil || s != nil {
		t.Fatal("disabled profiler opened listener")
	}
	for _, addr := range []string{":6060", "0.0.0.0:6060", "[::]:6060", "example.com:6060", "localhost:70000"} {
		if _, err := Start(t.Context(), addr, ":18080", 0, nil, nil); err == nil {
			t.Fatal("accepted non-loopback or invalid address", addr)
		}
	}
}
