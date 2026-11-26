// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

//go:generate go tool optgen generate

import (
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/messaging/broker/msg"
	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/reliability/idempotency"
)

// Default constants for backoff and release timeout.
const (
	// DefaultBackOffBase is the base delay for exponential backoff on busy or error.
	DefaultBackOffBase = 1 * time.Second

	// DefaultBackOffMax is the maximum delay for exponential backoff.
	DefaultBackOffMax = 1 * time.Minute

	// DefaultBackOffJitter is the randomized jitter fraction (0.0 to 1.0).
	DefaultBackOffJitter = 0.2

	// DefaultMaxKeyLength bounds an idempotency key, in bytes: a longer one terminates its message.
	DefaultMaxKeyLength = 128

	// defaultReleaseTimeout is the maximum time budgeted for Complete/Release calls.
	defaultReleaseTimeout = 5 * time.Second

	// completeMaxRetries, completeRetryBase and completeRetryMax bound the retries of Complete inside the release timeout.
	completeMaxRetries = 3
	completeRetryBase  = 100 * time.Millisecond
	completeRetryMax   = time.Second
)

var (
	// DefaultBusyBackOff is the default backoff function for IN_PROGRESS locks.
	DefaultBusyBackOff = msg.ExponentialBackOff(DefaultBackOffBase, DefaultBackOffMax, DefaultBackOffJitter)

	// DefaultErrorBackOff is the default backoff function on storage errors during AttemptLock.
	DefaultErrorBackOff = msg.ExponentialBackOff(DefaultBackOffBase, DefaultBackOffMax, DefaultBackOffJitter)
)

// options contains configuration fields for the broker idempotency middleware.
type options struct {
	keyFunc      KeyFunc         `optgen:"default=DefaultKeyFunc,notnil"`
	busyBackOff  msg.BackOffFunc `optgen:"default=DefaultBusyBackOff,notnil"`
	errorBackOff msg.BackOffFunc `optgen:"default=DefaultErrorBackOff,notnil"`
	// maxKeyLength bounds a key in bytes; a longer one terminates its message with ErrKeyTooLong. Non-positive is
	// DefaultMaxKeyLength.
	maxKeyLength int `optgen:"default=DefaultMaxKeyLength"`
	lockOpts     idempotency.AttemptLockOpts
	logger       *slog.Logger
	meter        metrics.Meter `optgen:"notnil"`
}
