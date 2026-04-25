// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import (
	"context"
	"log/slog"
	"time"
)

// RecoveryTimeoutMonitor checks for stale recovery marks and clears them
// via [Supervisor.ClearStaleRecoveries]. A recovery mark may become stale
// due to:
//
//   - Server restart during recovery process
//   - Code bug (panic, deadlock, etc.)
//   - Network issues preventing recovery completion
//
// Without cleanup, stale marks would block stream recovery permanently.
//
// The monitor is designed to be used with an external scheduler.
// Call [RecoveryTimeoutMonitor.Run] to perform a single cleanup cycle.
type RecoveryTimeoutMonitor struct {
	supervisor *Supervisor
	logger     *slog.Logger
	timeout    time.Duration
}

// RecoveryTimeoutMonitorConfig holds configuration for the RecoveryTimeoutMonitor.
type RecoveryTimeoutMonitorConfig struct {
	Supervisor *Supervisor
	Logger     *slog.Logger
	Timeout    time.Duration // How long before a recovery mark is considered stale
}

// NewRecoveryTimeoutMonitor creates a new RecoveryTimeoutMonitor.
func NewRecoveryTimeoutMonitor(cfg RecoveryTimeoutMonitorConfig) *RecoveryTimeoutMonitor {
	return &RecoveryTimeoutMonitor{
		supervisor: cfg.Supervisor,
		logger:     cfg.Logger,
		timeout:    cfg.Timeout,
	}
}

// Run performs a single stale recovery cleanup cycle.
// Compatible with scheduler.TaskFunc signature for use with external scheduler.
func (t *RecoveryTimeoutMonitor) Run(_ context.Context) error {
	t.cleanupStale()
	return nil
}

// cleanupStale clears any recovery marks that are older than the timeout.
func (t *RecoveryTimeoutMonitor) cleanupStale() {
	cleared := t.supervisor.ClearStaleRecoveries(t.timeout)

	if len(cleared) > 0 {
		t.logger.Info("cleared stale recovery marks",
			slog.Int("count", len(cleared)),
			slog.Any("streams", cleared))
	}
}

// GetTimeout returns the configured timeout duration.
func (t *RecoveryTimeoutMonitor) GetTimeout() time.Duration {
	return t.timeout
}
