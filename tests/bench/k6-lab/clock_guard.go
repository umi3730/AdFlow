package main

import (
	"sync"
	"time"
)

type clockObservation struct {
	StartedAt          time.Time `json:"startedAt"`
	Samples            int       `json:"samples"`
	MaxAbsoluteDriftMS float64   `json:"maxAbsoluteDriftMs"`
	Stable             bool      `json:"stable"`
}
type clockGuard struct {
	start       time.Time
	stop, done  chan struct{}
	once        sync.Once
	observation clockObservation
}

func clockDrift(wall, monotonic time.Duration) time.Duration {
	d := wall - monotonic
	if d < 0 {
		return -d
	}
	return d
}
func newClockGuard() *clockGuard {
	g := &clockGuard{start: time.Now(), stop: make(chan struct{}), done: make(chan struct{})}
	g.observation = clockObservation{StartedAt: g.start.UTC(), Stable: true}
	go func() {
		defer close(g.done)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				g.sample()
			case <-g.stop:
				g.sample()
				return
			}
		}
	}()
	return g
}
func (g *clockGuard) sample() {
	now := time.Now()
	d := clockDrift(time.Duration(now.UnixNano()-g.start.UnixNano()), now.Sub(g.start))
	g.observation.Samples++
	g.observation.MaxAbsoluteDriftMS = max(g.observation.MaxAbsoluteDriftMS, float64(d)/float64(time.Millisecond))
	if d > 50*time.Millisecond {
		g.observation.Stable = false
	}
}
func (g *clockGuard) Stop() clockObservation {
	g.once.Do(func() { close(g.stop) })
	<-g.done
	return g.observation
}
