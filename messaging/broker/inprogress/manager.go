// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package inprogress

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/altessa-s/go-atlas-core/collections/slices"
	"github.com/altessa-s/go-atlas-core/runtime/concurrency"
	"github.com/altessa-s/go-atlas-core/runtime/cycle"
	"github.com/altessa-s/go-atlas-core/runtime/panics"

	schedtask "github.com/altessa-s/go-horai/task"
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
	sending  atomic.Bool // an InProgress call is outstanding
	interval time.Duration
}

// Manager manages periodic InProgress heartbeat sending for all registered [Heartbeater] values.
// Tasks are executed via [Manager.RunTickCycle] which should be registered with a scheduler.
//
// All exported methods are safe for concurrent use.
type Manager struct {
	mu        sync.Mutex
	entries   map[uint64]*entry
	nextID    uint64
	logger    *slog.Logger
	metrics   *inprogressMetrics
	scheduler schedtask.Registrar
	tickTask  *cycle.Guard // Guards RunTickCycle and marks scheduler management.

	heartbeatTimeout time.Duration
}

// New creates a new Manager with the given options. It returns an error when
// a configured scheduler rejects the heartbeat task: without the task a
// handler running past AckWait would be redelivered concurrently.
//
// Example:
//
//	mgr, err := inprogress.New(
//	    inprogress.WithLogger(logger),
//	)
//	if err != nil {
//	    return err
//	}
//	sched.Register(ctx, schedtask.Config{
//	    ID:       "inprogress-heartbeat",
//	    Schedule: "@every 1s",
//	    Func:     mgr.RegisterTickSchedulerFunc(),
//	})
func New(opts ...Option) (*Manager, error) {
	cfg := newOptions(opts...)

	mgr := &Manager{
		entries:   make(map[uint64]*entry),
		logger:    cfg.logger,
		metrics:   newInprogressMetrics(cfg.meter),
		scheduler: cfg.scheduler,

		heartbeatTimeout: cfg.heartbeatTimeout,
	}
	mgr.tickTask = cycle.New(mgr.runTickCycleInternal)

	if err := mgr.registerTickTask(cfg); err != nil {
		return nil, err
	}

	return mgr, nil
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
// holding the lock to avoid blocking Register/stop during network I/O. They are
// sent concurrently, each bounded by the heartbeat timeout, so a slow call
// does not delay the others past their AckWait.
func (m *Manager) tick(ctx context.Context) error {
	now := time.Now()

	m.mu.Lock()
	snapshot := make([]*entry, 0, len(m.entries))
	for _, e := range m.entries {
		snapshot = slices.AppendIf(snapshot, now.Sub(time.Unix(0, e.lastSent.Load())) >= e.interval, e)
	}
	m.mu.Unlock()

	return concurrency.Process(ctx, snapshot, m.heartbeat)
}

// heartbeat sends one InProgress and waits for it for at most the heartbeat
// timeout. [Heartbeater.InProgress] takes no context, so a call that outlives
// the timeout is abandoned, not canceled; the entry is skipped until that
// call returns, which keeps a stuck heartbeater at one outstanding call.
func (m *Manager) heartbeat(ctx context.Context, e *entry) error {
	if e.stopped.Load() || !e.sending.CompareAndSwap(false, true) {
		return nil
	}
	e.lastSent.Store(time.Now().UnixNano())

	done := make(chan error, 1)
	go func() {
		defer e.sending.Store(false)
		defer panics.Handle(ctx)
		done <- e.h.InProgress()
	}()

	timeout := time.NewTimer(m.heartbeatTimeout)
	defer timeout.Stop()

	select {
	case err := <-done:
		if err != nil {
			m.metrics.heartbeatErrors.Inc()
			m.logger.Warn("failed to send InProgress", slog.Any("error", err))
		} else {
			m.metrics.heartbeatsSent.Inc()
		}
	case <-timeout.C:
		m.metrics.heartbeatErrors.Inc()
		m.logger.Warn("InProgress timed out", slog.Duration("timeout", m.heartbeatTimeout))
	case <-ctx.Done():
	}
	return nil
}
