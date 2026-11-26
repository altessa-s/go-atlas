// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natskvlease

//go:generate go tool optgen generate --type=leaseOptions --option-type=LeaseOption --option-prefix=Lease --output=lease_options_gen.go
//go:generate go tool optgen generate --type=retryOptions --option-type=RetryOption --option-prefix=Retry --output=retry_options_gen.go

import (
	"log/slog"
	"time"
)

// leaseOptions holds the optional settings of a [Lease]. The key, lifetime and
// initial value are mandatory and are passed to [NewLease] directly.
type leaseOptions struct {
	// WithLeaseRenewRatio sets the fraction of the lease lifetime at which the
	// lease is renewed (0.5 renews at half the lifetime). A value outside
	// (0, 1], the zero default included, falls back to [DefaultRenewRatio].
	renewRatio float64 `optval:"nonpositive"`

	// WithLeaseOwnerCheck sets the function reporting whether a stored value
	// belongs to this lease holder. Use a stable owner check when the payload
	// changes. By default renewal and cleanup compare the whole payload.
	ownerCheck func(value []byte) bool

	// WithLeaseCallbacks sets the callbacks fired on lease state changes.
	callbacks LeaseCallbacks

	// WithLeaseLogger sets the logger for lease operations; nil uses
	// [slog.Default].
	logger *slog.Logger `optgen:"default=slog.Default()"`

	// WithLeaseObserver reports lease operations and held-state changes to the observer.
	// A nil observer disables reporting.
	observer LeaseObserver
}

// retryOptions holds the settings of one [Retry] call.
type retryOptions struct {
	// WithRetryMaxAttempts sets the total number of attempts, the first call
	// included. Zero behaves like one: the function is called exactly once.
	maxAttempts uint `optgen:"default=DefaultMaxRetries" optval:"nonpositive"`

	// WithRetryMaxElapsedTime sets the total time budget across all attempts.
	maxElapsedTime time.Duration `optgen:"default=DefaultMaxElapsedTime" optval:"nonpositive"`

	// WithRetryJitter sets the maximum fraction of each computed delay added as
	// randomization (0.0–1.0). Zero produces deterministic, lockstep retries.
	jitter float64 `optgen:"default=DefaultJitter" optval:"nonpositive"`
}
