// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dlock

//go:generate go tool optgen generate

import (
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// DefaultHealthServiceName is the name under which DLock registers itself
// with the health coordinator when no override is supplied.
const DefaultHealthServiceName = "dlock"

// DefaultReleaseTimeout bounds the release Synchronize performs once its
// function returned.
const DefaultReleaseTimeout = 10 * time.Second

// options contains DLock configuration.
//
// There is deliberately no acquire timeout here. Acquisition is a single
// attempt that fails fast, and the only way DLock could bound it would be
// through the context it passes to the provider — the same context that scopes
// the lock's lifetime, so the bound would release the lock the moment it
// elapsed. The provider owns that bound instead; see the NATS provider's
// WithAcquireTimeout.
type options struct {
	logger *slog.Logger
	meter  metrics.Meter `optgen:"notnil"`
	// healthCoordinator registers DLock with a health coordinator on
	// construction. Disabled when nil.
	healthCoordinator health.Registrar `optgen:"notnil"`
	// healthServiceName customizes the service name used for health
	// registration. Useful when multiple DLock instances share one
	// coordinator (e.g. "dlock-payments", "dlock-inventory").
	healthServiceName string `optgen:"default=DefaultHealthServiceName"`
	// releaseTimeout bounds the release Synchronize performs once its
	// function returned; 0 leaves it unbounded.
	releaseTimeout time.Duration `optgen:"default=DefaultReleaseTimeout"`
}
