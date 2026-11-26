// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/altessa-s/go-atlas-core/runtime/cycle"
	"github.com/altessa-s/go-atlas-core/types/nilcheck"

	corectx "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	schedtask "github.com/altessa-s/go-horai/task"
)

type checkScheduler struct {
	scheduler      schedtask.Registrar
	checkSchedule  string
	registrationMu sync.Mutex
	guard          *cycle.Guard
	closed         atomic.Bool
	metrics        *healthMetrics
	logger         *slog.Logger
	statuses       *statusReporter
	watchers       *subscriptions
}

// RegisterHealthChecks installs the configured periodic check. Failures leave
// manual checks available; callers may retry. New does not register tasks.
func (c *Coordinator) RegisterHealthChecks(ctx context.Context) error {
	return c.checks.register(ctx)
}

func (s *checkScheduler) register(ctx context.Context) error {
	s.registrationMu.Lock()
	defer s.registrationMu.Unlock()
	if s.guard.IsManaged() {
		return nil
	}
	if nilcheck.IsNil(s.scheduler) || s.checkSchedule == "" {
		return nil
	}

	ctx, cancel := corectx.WithMaxTimeout(ctx, DefaultCheckTimeout)
	defer cancel()
	taskCfg := schedtask.Config{
		ID:             "health-check",
		Description:    "Periodic health check cycle for all watched services",
		Func:           s.guard.Entry(),
		Schedule:       s.checkSchedule,
		Priority:       schedtask.PriorityNormal,
		RunOnStart:     true,
		Unmanaged:      true,
		DisableHistory: true,
	}

	if err := s.scheduler.Register(ctx, taskCfg); err != nil {
		return coreerrs.WrapOperation(err, "register health check task")
	}

	s.guard.MarkManaged()
	s.logger.Debug("registered health check task",
		slog.String("schedule", s.checkSchedule))

	return nil
}

// RegisterHealthCheckSchedulerFunc returns a function for use by a scheduler and marks
// health check as scheduler-managed. After calling this method, direct calls to
// RunHealthCheckCycle will return ErrSchedulerManaged.
func (c *Coordinator) RegisterHealthCheckSchedulerFunc() func(context.Context) error {
	c.checks.registrationMu.Lock()
	defer c.checks.registrationMu.Unlock()
	return c.checks.guard.Managed()
}

// RunHealthCheckCycle performs a single health check cycle for all watched services.
// This method is designed to be called manually for one-time health check.
// If the function is registered with a scheduler, this method returns ErrSchedulerManaged.
func (c *Coordinator) RunHealthCheckCycle(ctx context.Context) error {
	err := c.checks.guard.Run(ctx)
	if errors.Is(err, cycle.ErrManaged) {
		return ErrSchedulerManaged
	}
	return err
}

func (s *checkScheduler) run(ctx context.Context) error {
	if s.closed.Load() {
		return nil
	}

	stop := s.metrics.checkCycleDuration.Start()
	defer stop()

	watchedServices := make(map[string][]poll)
	s.watchers.collectPolls(watchedServices)

	// Check health for each watched service and notify watchers if changed
	for service, watchers := range watchedServices {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		currentStatus := s.statuses.getHealthStatus(ctx, service)
		s.metrics.checksPerformed.Inc()

		for _, target := range watchers {
			if previous, sent := target.watcher.notifyPoll(currentStatus, target.pushes); sent {
				s.metrics.statusChanges.Inc()
				s.logger.Info("health status changed",
					slog.String("service", service),
					slog.String("from", previous.String()),
					slog.String("to", currentStatus.String()))
			}
		}
	}

	return nil
}
