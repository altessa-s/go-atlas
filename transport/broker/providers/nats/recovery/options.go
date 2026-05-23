// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"log/slog"
	"time"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// Default configuration values for recovery manager.
const (
	// DefaultHealthCheckInterval is the default interval for periodic health checks.
	// Acts as fallback detection when advisory events are missed.
	DefaultHealthCheckInterval = 5 * time.Minute

	// DefaultMaxRecoveryAttempts is the default maximum number of recovery attempts.
	DefaultMaxRecoveryAttempts = 3

	// DefaultRecoveryBackoff is the default base backoff duration between recovery attempts.
	// Actual backoff is calculated as backoff * attemptNumber.
	DefaultRecoveryBackoff = 5 * time.Second

	// DefaultStaleRecoveryTimeout is the default timeout after which a recovery mark is
	// considered stale and will be cleared.
	DefaultStaleRecoveryTimeout = 20 * time.Minute

	// DefaultStaleRecoveryCheckInterval is the default interval for checking stale recovery marks.
	DefaultStaleRecoveryCheckInterval = 1 * time.Minute
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

// WithOnRecoverySuccess sets the callback for successful recovery.
func WithOnRecoverySuccess(cb RecoverySuccessCallback) Option {
	return func(o *options) {
		o.onRecoverySuccess = cb
	}
}

// WithOnRecoveryFailure sets the callback for failed recovery.
func WithOnRecoveryFailure(cb RecoveryFailureCallback) Option {
	return func(o *options) {
		o.onRecoveryFailure = cb
	}
}

// WithOnManualRecoveryNeeded sets the callback for manual recovery notification.
func WithOnManualRecoveryNeeded(cb ManualRecoveryCallback) Option {
	return func(o *options) {
		o.onManualRecoveryNeeded = cb
	}
}

// WithOnStaleRecoveryCleared sets the callback for stale recovery cleanup.
func WithOnStaleRecoveryCleared(cb StaleRecoveryClearedCallback) Option {
	return func(o *options) {
		o.onStaleRecoveryCleared = cb
	}
}

// options contains configuration fields for the recovery Manager.
type options struct {
	// healthCheckInterval defines how often the health monitor checks registered streams/consumers.
	healthCheckInterval time.Duration `optgen:"default=DefaultHealthCheckInterval"`

	// maxRecoveryAttempts defines the maximum number of recovery attempts before giving up.
	maxRecoveryAttempts int `optgen:"default=DefaultMaxRecoveryAttempts" optval:"positive"`

	// recoveryBackoff defines the base backoff duration between recovery attempts.
	recoveryBackoff time.Duration `optgen:"default=DefaultRecoveryBackoff"`

	// staleRecoveryTimeout defines how long a recovery mark can exist before being cleared.
	staleRecoveryTimeout time.Duration `optgen:"default=DefaultStaleRecoveryTimeout"`

	// staleRecoveryCheckInterval defines how often to check for stale recovery marks.
	staleRecoveryCheckInterval time.Duration `optgen:"default=DefaultStaleRecoveryCheckInterval"`

	// onRecoverySuccess is called when a stream or consumer is successfully recovered.
	// Parameters: stream name, consumer name (empty for stream-only recovery).
	onRecoverySuccess func(stream, consumer string) `optgen:"manual"`

	// onRecoveryFailure is called when recovery fails after all attempts.
	// Parameters: stream name, consumer name, error.
	onRecoveryFailure func(stream, consumer string, err error) `optgen:"manual"`

	// onManualRecoveryNeeded is called for streams with RecoveryStrategyManual.
	// Parameters: stream name, advisory event (may be nil).
	onManualRecoveryNeeded func(stream string, event any) `optgen:"manual"`

	// onStaleRecoveryCleared is called when a stale recovery mark is cleared.
	// Parameter: stream name.
	onStaleRecoveryCleared func(stream string) `optgen:"manual"`

	// logger is the logger for recovery operations.
	logger *slog.Logger

	// Scheduler configuration
	scheduler                    corescheduler.TaskRegistrar `optgen:"notnil"`
	healthCheckSchedule          string
	staleRecoveryCleanupSchedule string
}
