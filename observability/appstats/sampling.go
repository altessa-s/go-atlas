// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package appstats

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	corecontext "github.com/altessa-s/go-atlas-core/context"
)

type samplingLoop struct {
	mu         sync.Mutex
	running    atomic.Bool
	stopChan   chan struct{}
	initialize func()
	sample     func(context.Context)
}

var sampler = samplingLoop{initialize: cachedMetrics.initialize, sample: collectMetrics}

// StartMetricsCollection initializes the background metrics collection system.
// It starts a goroutine that periodically collects CPU, memory, and network metrics
// using atomic operations for lock-free access. This function is idempotent and
// safe to call multiple times.
//
// The background collection runs every DefaultCacheInterval (2 seconds) and provides
// cached metrics that can be accessed with sub-microsecond latency.
func StartMetricsCollection() {
	//nolint:contextcheck // Convenience wrapper; use StartMetricsCollectionWithContext when you have an inherited ctx.
	StartMetricsCollectionWithContext(context.Background())
}

// StartMetricsCollectionWithContext initializes the background metrics collection system
// using ctx as the base context for gopsutil calls (timeouts are still applied per tick).
//
// Note: canceling ctx stops the collection, as [StopMetricsCollection] does; a later
// start begins a new one. Prefer passing a long-lived application context.
//
//nolint:contextcheck // Background collector is not request-scoped; ctx here is a best-effort base for gopsutil calls.
func StartMetricsCollectionWithContext(ctx context.Context) {
	sampler.start(ctx)
}

func (s *samplingLoop) start(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running.Load() {
		return
	}

	ctx = corecontext.OrBackground(ctx)

	stop := make(chan struct{})
	s.stopChan = stop
	s.running.Store(true)
	s.initialize()
	go s.run(ctx, stop)
}

// StopMetricsCollection stops the background metrics collection system.
// It signals the background goroutine to terminate and stops all metric updates.
// This function is idempotent and safe to call multiple times.
//
// After calling this function, cached metrics will become stale and
// IsCacheStale() will return true after MaxCacheAge duration.
func StopMetricsCollection() {
	sampler.stop()
}

func (s *samplingLoop) stop() {
	s.mu.Lock()
	if !s.running.Load() {
		s.mu.Unlock()
		return
	}
	stop := s.stopChan
	s.running.Store(false)
	s.stopChan = nil
	s.mu.Unlock()

	if stop != nil {
		close(stop)
	}
}

func (s *samplingLoop) run(baseCtx context.Context, stop <-chan struct{}) {
	ticker := time.NewTicker(DefaultCacheInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Bound the gopsutil calls so we don't risk blocking forever in the background.
			ctx, cancel := corecontext.WithMaxTimeout(baseCtx, DefaultCacheInterval)
			s.sample(ctx)
			cancel()
		case <-stop:
			return
		case <-baseCtx.Done():
			// The collection cannot outlive its base context. Mark it stopped,
			// unless a newer collection has replaced this one, so a later
			// start is not a no-op.
			s.mu.Lock()
			if s.stopChan == stop {
				s.running.Store(false)
				s.stopChan = nil
			}
			s.mu.Unlock()
			return
		}
	}
}
