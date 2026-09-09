package profiling

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type InstanceInfo struct {
	APIAddress       string      `json:"apiAddress"`
	PID              int         `json:"pid"`
	StartedAt        time.Time   `json:"startedAt"`
	BlockProfileRate int         `json:"blockProfileRate"`
	GOMAXPROCS       int         `json:"gomaxprocs"`
	Pool             sql.DBStats `json:"pool"`
}

type Server struct {
	listener net.Listener
	done     chan struct{}
}

func (s *Server) Address() string       { return s.listener.Addr().String() }
func (s *Server) Done() <-chan struct{} { return s.done }

func ValidateAddress(address string) error {
	if address == "" {
		return nil
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid profiling address: %w", err)
	}
	ip := net.ParseIP(host)
	if !strings.EqualFold(host, "localhost") && (ip == nil || !ip.IsLoopback()) {
		return errors.New("profiling address must use localhost or a loopback IP")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 0 || n > 65535 {
		return errors.New("invalid profiling port")
	}
	return nil
}

// Start is disabled by an empty address. Each process owns its own mux/listener.
// Instance metadata intentionally excludes DSNs, environment values and tokens.
func Start(ctx context.Context, address, apiAddress string, blockRate int, stats func() sql.DBStats, logger *slog.Logger) (*Server, error) {
	if err := ValidateAddress(address); err != nil {
		return nil, err
	}
	if blockRate < 0 {
		return nil, errors.New("block profile rate cannot be negative")
	}
	if address == "" {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{listener: listener, done: make(chan struct{})}
	started := time.Now().UTC()
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.HandleFunc("/debug/instance", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		info := InstanceInfo{APIAddress: apiAddress, PID: os.Getpid(), StartedAt: started, BlockProfileRate: blockRate, GOMAXPROCS: runtime.GOMAXPROCS(0)}
		if stats != nil {
			info.Pool = stats()
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(info)
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 3 * time.Second, IdleTimeout: 30 * time.Second}
	if blockRate > 0 {
		runtime.SetBlockProfileRate(blockRate)
	}
	go func() {
		defer close(s.done)
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("profiling server failed", "error", err)
		}
	}()
	go func() {
		select {
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := server.Shutdown(shutdown); err != nil {
				_ = server.Close()
			}
		case <-s.done:
		}
		if blockRate > 0 {
			runtime.SetBlockProfileRate(0)
		}
	}()
	return s, nil
}
