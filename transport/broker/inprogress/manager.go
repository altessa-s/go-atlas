// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package inprogress

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// Heartbeater is the interface for types that support InProgress heartbeats.
// [msg.Message] satisfies this interface via [msg.Message.InProgress].
type Heartbeater interface {
	InProgress() error
}

// entry represents a registered heartbeater with its send interval.
type entry struct {
	h        Heartbeater
	lastSent atomic.Int64 // unix nano
	stopped  atomic.Bool
	interval time.Duration
}

// Manager manages periodic InProgress heartbeat sending for all registered [Heartbeater] values.
// Tasks are executed via [Manager.RunTickCycle] which should be registered with a scheduler.
//
// All exported methods are safe for concurrent use.
type Manager struct {
	mu          sync.Mutex
	entries     map[uint64]*entry
	nextID      uint64
	logger      *slog.Logger
	metrics     *inprogressMetrics
	scheduler   corescheduler.TaskRegistrar
	tickRunning atomic.Bool // Guards against concurrent RunTickCycle calls.

	schedulerTickRegistered atomic.Bool // Marks if RunTickCycle is managed by scheduler.
}

// New creates a new Manager with the given options.
//
// Example:
//
//	mgr := inprogress.New(
//	    inprogress.WithLogger(logger),
//	)
//	sched.Register(ctx, corescheduler.TaskConfig{
//	    ID:       "inprogress-heartbeat",
//	    Interval: 500 * time.Millisecond,
//	    Func:     mgr.RunTickCycle,
//	})
func New(opts ...Option) *Manager {
	cfg := newOptions(opts...)

	mgr := &Manager{
		entries:   make(map[uint64]*entry),
		logger:    cfg.logger,
		metrics:   newInprogressMetrics(cfg.collector),
		scheduler: cfg.scheduler,
	}

	// Register tick cycle task with scheduler if provided
	if err := mgr.registerTickTask(cfg); err != nil {
		// Log error but don't fail creation - task registration is optional
		cfg.logger.Warn("failed to register tick cycle task", slog.Any("error", err))
	}

	return mgr
}

// Register adds a heartbeater to the manager with the given interval.
// The heartbeater's InProgress method will be called approximately every interval.
// Returns a stop function that removes the heartbeater when called.
// The stop function is safe to call multiple times.
//
// Example:
//
//	stop := mgr.Register(message, 5*time.Second)
//	defer stop()
func (m *Manager) Register(h Heartbeater, interval time.Duration) (stop func()) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := m.nextID
	m.nextID++

	e := &entry{
		h:        h,
		interval: interval,
	}
	e.lastSent.Store(time.Now().UnixNano())

	m.entries[id] = e

	var once sync.Once
	return func() {
		once.Do(func() {
			e.stopped.Store(true)
			m.mu.Lock()
			defer m.mu.Unlock()
			delete(m.entries, id)
		})
	}
}

// tick checks all registered heartbeaters and sends InProgress for those due.
// A snapshot of due entries is taken under the lock, then heartbeats are sent without
// holding the lock to avoid blocking Register/stop during network I/O.
func (m *Manager) tick() {
	now := time.Now()
	nowNano := now.UnixNano()

	m.mu.Lock()
	snapshot := make([]*entry, 0, len(m.entries))
	for _, e := range m.entries {
		if now.Sub(time.Unix(0, e.lastSent.Load())) >= e.interval {
			snapshot = append(snapshot, e)
		}
	}
	m.mu.Unlock()

	for _, e := range snapshot {
		if e.stopped.Load() {
			continue
		}
		e.lastSent.Store(nowNano)
		if err := e.h.InProgress(); err != nil {
			m.metrics.heartbeatErrors.Inc()
			m.logger.Warn("failed to send InProgress", slog.Any("error", err))
		} else {
			m.metrics.heartbeatsSent.Inc()
		}
	}
}
