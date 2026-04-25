// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ocsp

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"

	"github.com/altessa-s/go-atlas/core/types/nilcheck"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// registerSchedulerTask registers the OCSP refresh task with the scheduler if configured.
func (s *Stapler) registerSchedulerTask(opts *options) error {
	if nilcheck.IsNil(s.scheduler) || opts.refreshSchedule == "" {
		return nil
	}

	ctx := context.Background()
	taskCfg := corescheduler.TaskConfig{
		ID:          "ocsp-stapler-refresh-all",
		Description: "Refresh all cached OCSP staples",
		Func:        s.RegisterRefreshAllSchedulerFunc(),
		Schedule:    opts.refreshSchedule,
		Priority:    corescheduler.TaskPriorityLow,
	}

	if err := s.scheduler.Register(ctx, taskCfg); err != nil {
		return coreerrs.WrapOperation(err, "register OCSP refresh task")
	}

	s.logger.Debug("registered OCSP refresh task",
		slog.String("schedule", opts.refreshSchedule))

	return nil
}

// RegisterRefreshAllSchedulerFunc returns a function for use by a scheduler and marks
// refresh as scheduler-managed. After calling this method, direct calls to
// RunRefreshAll will return ErrSchedulerManaged.
func (s *Stapler) RegisterRefreshAllSchedulerFunc() func(context.Context) error {
	s.schedulerRefreshAllRegistered.Store(true)
	return s.runRefreshAllInternal
}

// RunRefreshAll refreshes all OCSP responses in the cache that need it.
// This method is designed to be called manually for one-time refresh.
// If the function is registered with a scheduler, this method returns ErrSchedulerManaged.
func (s *Stapler) RunRefreshAll(ctx context.Context) error {
	if s.schedulerRefreshAllRegistered.Load() {
		return ErrSchedulerManaged
	}
	return s.runRefreshAllInternal(ctx)
}

// runRefreshAllInternal performs the actual refresh of all OCSP responses.
// It is safe to call concurrently; if already running, returns immediately.
func (s *Stapler) runRefreshAllInternal(ctx context.Context) error {
	// Prevent concurrent execution
	if !s.refreshAllRunning.CompareAndSwap(false, true) {
		return nil // Already running, skip this cycle
	}
	defer s.refreshAllRunning.Store(false)

	s.mu.RLock()
	// Collect all entries to avoid holding the lock while fetching
	type refreshItem struct {
		cert *tls.Certificate
	}
	var items []refreshItem
	for _, entry := range s.cache {
		if entry.cert != nil {
			items = append(items, refreshItem{cert: entry.cert})
		}
	}
	s.mu.RUnlock()

	var errs []error
	for _, item := range items {
		if err := s.RunRefreshCycle(ctx, item.cert); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}
