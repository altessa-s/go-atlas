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

// NewRecoveryTimeoutMonitor creates a new RecoveryTimeoutMonitor that clears
// stale recovery marks of supervisor.
func NewRecoveryTimeoutMonitor(supervisor *Supervisor, opts ...RecoveryTimeoutMonitorOption) *RecoveryTimeoutMonitor {
	cfg := newRecoveryTimeoutMonitorOptions(opts...)

	return &RecoveryTimeoutMonitor{
		supervisor: supervisor,
		logger:     cfg.logger,
		timeout:    cfg.timeout,
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
