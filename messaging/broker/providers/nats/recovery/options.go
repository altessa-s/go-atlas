// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

//go:generate go tool optgen generate --type=options --option-type=Option --output=options_gen.go
//go:generate go tool optgen generate --type=streamOptions --option-type=StreamOption --output=stream_options_gen.go --skip-option-type

import (
	"log/slog"
	"time"

	schedtask "github.com/altessa-s/go-horai/task"
)

// Default configuration values for recovery manager.
const (
	// DefaultMaxRecoveryAttempts is the default maximum number of recovery attempts.
	DefaultMaxRecoveryAttempts = 3

	// DefaultRecoveryBackoff is the default base backoff duration between recovery attempts.
	// Actual backoff is calculated as backoff * attemptNumber.
	DefaultRecoveryBackoff = 5 * time.Second

	// DefaultStaleRecoveryTimeout is the default timeout after which a recovery mark is
	// considered stale and will be cleared.
	DefaultStaleRecoveryTimeout = 20 * time.Minute
)

// RecoverySuccessCallback is invoked after a stream or consumer is successfully recovered.
// The consumer parameter is empty for stream-level recovery.
type RecoverySuccessCallback func(stream, consumer string)

// RecoveryFailureCallback is invoked when recovery fails after all retry attempts.
// The consumer parameter is empty for stream-level recovery.
type RecoveryFailureCallback func(stream, consumer string, err error)

// ManualRecoveryCallback is invoked for streams configured with [RecoveryStrategyManual].
// The event parameter carries the advisory payload (may be nil for health-check triggers).
type ManualRecoveryCallback func(stream string, event any)

// StaleRecoveryClearedCallback is invoked when a recovery mark exceeds the stale timeout
// and is cleared to allow future recovery attempts.
type StaleRecoveryClearedCallback func(stream string)

// options contains configuration fields for the recovery Manager.
type options struct {
	// maxRecoveryAttempts defines the maximum number of recovery attempts before giving up.
	maxRecoveryAttempts int `optgen:"default=DefaultMaxRecoveryAttempts" optval:"positive"`

	// recoveryBackoff defines the base backoff duration between recovery attempts.
	recoveryBackoff time.Duration `optgen:"default=DefaultRecoveryBackoff"`

	// staleRecoveryTimeout defines how long a recovery mark can exist before being cleared.
	staleRecoveryTimeout time.Duration `optgen:"default=DefaultStaleRecoveryTimeout"`

	// WithOnRecoverySuccess sets the callback for successful recovery.
	onRecoverySuccess RecoverySuccessCallback `opt:"OnRecoverySuccess" optgen:"allownull"`

	// WithOnRecoveryFailure sets the callback for failed recovery.
	onRecoveryFailure RecoveryFailureCallback `opt:"OnRecoveryFailure" optgen:"allownull"`

	// WithOnManualRecoveryNeeded sets the callback for manual recovery notification.
	onManualRecoveryNeeded ManualRecoveryCallback `opt:"OnManualRecoveryNeeded" optgen:"allownull"`

	// WithOnStaleRecoveryCleared sets the callback for stale recovery cleanup.
	onStaleRecoveryCleared StaleRecoveryClearedCallback `opt:"OnStaleRecoveryCleared" optgen:"allownull"`

	// logger is the logger for recovery operations.
	logger *slog.Logger

	// Scheduler configuration
	scheduler                    schedtask.Registrar `optgen:"notnil"`
	healthCheckSchedule          string
	staleRecoveryCleanupSchedule string
	// healthCheckTaskID is the scheduler task ID of the health check. The
	// scheduler keys tasks by ID, so managers sharing an ID leave only the
	// last one running. Empty derives a unique ID per manager instance,
	// "broker-recovery-health-check-<n>"; set it for an ID that is stable
	// across restarts.
	healthCheckTaskID string
	// staleCleanupTaskID is the scheduler task ID of the stale recovery
	// cleanup, with the same rules as healthCheckTaskID; the derived default
	// is "broker-recovery-stale-cleanup-<n>".
	staleCleanupTaskID string
}
