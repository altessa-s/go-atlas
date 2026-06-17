// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package saga

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"context"
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/core/encoding/serializer"
	"github.com/altessa-s/go-atlas/observability/metrics"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// Default values for the orchestrator options.
const (
	// DefaultStepTimeout bounds a single invocation of a step action or
	// compensation when the step does not override it.
	DefaultStepTimeout = 30 * time.Second
	// DefaultMaxStepAttempts is the total number of attempts for a forward
	// step action (including the first) before it is considered failed.
	DefaultMaxStepAttempts = 3
	// DefaultStepRetryBaseDelay is the first backoff delay between step retries.
	DefaultStepRetryBaseDelay = 100 * time.Millisecond
	// DefaultStepRetryMaxDelay caps the exponential backoff between step retries.
	DefaultStepRetryMaxDelay = 5 * time.Second
	// DefaultMaxCompensationAttempts is the total number of attempts for a
	// single compensation before the saga is moved to the Failed status.
	DefaultMaxCompensationAttempts = 5
	// DefaultRecoverySchedule is the cron expression used to register the
	// background recovery cycle with a scheduler.
	DefaultRecoverySchedule = "@every 1m"
	// DefaultRecoveryBatchSize is the maximum number of recoverable instances
	// processed per recovery cycle.
	DefaultRecoveryBatchSize = 100
	// DefaultRecoveryTaskID is the scheduler task ID for the recovery cycle.
	DefaultRecoveryTaskID = "saga-recovery"
)

// DeadLetterFunc is invoked when a saga reaches the terminal [StatusFailed]
// state — a compensation exhausted its retries, or a post-pivot stage could
// not roll forward. The instance is passed for inspection and routing to a
// dead-letter queue or alert. It must not block for long.
type DeadLetterFunc func(inst *Instance)

// LeaderElector is the narrow leader-election contract consumed by the
// orchestrator to gate the background recovery cycle so that, in a multi-node
// deployment, only the elected leader scans the store. When set via
// [WithLeaderElector], the recovery cycle is a no-op on nodes whose IsLeader
// reports false. Gating is an optimization, not a correctness requirement: the
// store's optimistic-concurrency check already makes concurrent recovery cycles
// safe; the gate only avoids redundant scans and writes. A *leadelect.Leader
// satisfies this interface.
type LeaderElector interface {
	// IsLeader reports whether this node should run the recovery cycle now.
	IsLeader() bool
}

// options holds the orchestrator configuration. Configure it through the
// generated With* constructors and the hand-written options below.
type options struct {
	logger     *slog.Logger
	collector  metrics.Collector     `optgen:"notnil"`
	serializer serializer.Serializer `optgen:"notnil"`

	stepTimeout             time.Duration `optgen:"default=DefaultStepTimeout"`
	sagaTimeout             time.Duration // 0 → no deadline (auto-rollback disabled)
	maxStepAttempts         int           `optgen:"default=DefaultMaxStepAttempts"`
	stepRetryBaseDelay      time.Duration `optgen:"default=DefaultStepRetryBaseDelay"`
	stepRetryMaxDelay       time.Duration `optgen:"default=DefaultStepRetryMaxDelay"`
	maxCompensationAttempts int           `optgen:"default=DefaultMaxCompensationAttempts"`
	stepConcurrency         int           // 0 → core/runtime/concurrency default (IO-bound)

	recoverySchedule  string `optgen:"default=DefaultRecoverySchedule"`
	recoveryBatchSize int    `optgen:"default=DefaultRecoveryBatchSize"`
	recoveryTaskID    string `optgen:"default=DefaultRecoveryTaskID"`

	scheduler     corescheduler.TaskRegistrar `optgen:"notnil"`
	leaderElector LeaderElector               `optgen:"notnil"`
	baseCtx       context.Context             `opt:"Context" optgen:"notnil"`

	onDeadLetter DeadLetterFunc   `opt:"-"`
	shouldRetry  func(error) bool `opt:"-"`
}

// WithOnDeadLetter registers a callback invoked when a saga reaches the
// terminal [StatusFailed] state. Use it to alert or enqueue the instance for
// manual intervention. Passing nil clears any previously set hook.
func WithOnDeadLetter(fn DeadLetterFunc) Option {
	return func(o *options) {
		o.onDeadLetter = fn
	}
}

// WithShouldRetry installs a predicate that decides whether a step or
// compensation error is retryable. Returning false stops retries immediately
// for that error (the saga then compensates or fails as appropriate). Context
// cancellation is always treated as non-retryable regardless of this hook.
// Passing nil restores the default (retry every error).
func WithShouldRetry(fn func(error) bool) Option {
	return func(o *options) {
		o.shouldRetry = fn
	}
}
