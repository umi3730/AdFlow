package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/umi3730/adflow/internal/platform/profiling"
)

type poolSample struct {
	At       time.Time              `json:"at"`
	Instance profiling.InstanceInfo `json:"instance"`
}
type poolStage struct {
	base    string
	before  poolSample
	samples []poolSample
	cpu     chan error
	cpuFile string
	waited  bool
	cpuErr  error
}

func readPool(base string) (poolSample, error) {
	client := http.Client{Timeout: 2 * time.Second}
	response, err := client.Get(base + "/debug/instance")
	if err != nil {
		return poolSample{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return poolSample{}, fmt.Errorf("profiling identity status %d", response.StatusCode)
	}
	var info profiling.InstanceInfo
	if err = json.NewDecoder(response.Body).Decode(&info); err != nil {
		return poolSample{}, err
	}
	return poolSample{At: time.Now().UTC(), Instance: info}, nil
}

func (h *chainCapacity) beginPoolStage(id, label string) (*poolStage, error) {
	base, _ := h.report["profilingBase"].(string)
	if base == "" {
		return nil, nil
	}
	before, err := readPool(base)
	if err != nil {
		return nil, err
	}
	if before.Instance.APIAddress != strings.TrimPrefix(h.base, "http://") || before.Instance.Pool.MaxOpenConnections != h.report["poolMaxOpen"].(int) {
		return nil, fmt.Errorf("pool configuration/instance mismatch")
	}
	s := &poolStage{base: base, before: before}
	if label == "overload" {
		s.cpu = make(chan error, 1)
		s.cpuFile = id + "-cpu.pprof"
		go func() {
			client := http.Client{Timeout: 25 * time.Second}
			response, err := client.Get(base + "/debug/pprof/profile?seconds=20")
			if err != nil {
				s.cpu <- err
				return
			}
			defer response.Body.Close()
			if response.StatusCode != 200 {
				s.cpu <- fmt.Errorf("CPU profile status %d", response.StatusCode)
				return
			}
			data, err := io.ReadAll(response.Body)
			if err == nil {
				err = os.WriteFile(filepath.Join(h.dir, s.cpuFile), data, 0644)
			}
			s.cpu <- err
		}()
	}
	return s, nil
}

func (s *poolStage) sample() error {
	if s == nil {
		return nil
	}
	sample, err := readPool(s.base)
	if err != nil {
		return err
	}
	if sample.Instance.PID != s.before.Instance.PID || !sample.Instance.StartedAt.Equal(s.before.Instance.StartedAt) {
		return fmt.Errorf("API restarted during pool experiment")
	}
	s.samples = append(s.samples, sample)
	return nil
}
func (s *poolStage) waitCPU() error {
	if s == nil || s.cpu == nil {
		return nil
	}
	if !s.waited {
		s.cpuErr = <-s.cpu
		s.waited = true
	}
	return s.cpuErr
}
func (s *poolStage) finish() (map[string]any, error) {
	if s == nil {
		return nil, nil
	}
	if err := s.waitCPU(); err != nil {
		return nil, err
	}
	if err := s.sample(); err != nil {
		return nil, err
	}
	after := s.samples[len(s.samples)-1]
	peakInUse, peakOpen := 0, 0
	for _, sample := range s.samples {
		peakInUse = max(peakInUse, sample.Instance.Pool.InUse)
		peakOpen = max(peakOpen, sample.Instance.Pool.OpenConnections)
	}
	waits := after.Instance.Pool.WaitCount - s.before.Instance.Pool.WaitCount
	duration := after.Instance.Pool.WaitDuration - s.before.Instance.Pool.WaitDuration
	if waits < 0 || duration < 0 {
		return nil, fmt.Errorf("database wait counters decreased")
	}
	return map[string]any{"before": s.before, "after": after, "samples": s.samples, "sampledPeakInUse": peakInUse, "sampledPeakOpen": peakOpen, "waitCountDelta": waits, "waitDurationSeconds": duration.Seconds(), "cpuProfile": s.cpuFile}, nil
}
