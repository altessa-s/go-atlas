// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health

import (
	"errors"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Sentinel errors for health operations.
var (
	// ErrWatcherLimitExceeded is returned by [Coordinator.Subscribe] when the
	// per-service watcher limit ([WithMaxWatchersPerService]) is reached.
	ErrWatcherLimitExceeded = errors.New("health: watcher limit exceeded")

	// ErrServiceNotFound is returned when a requested service name is not
	// registered with the [Coordinator].
	ErrServiceNotFound = errors.New("health: service not found")

	// ErrCoordinatorShutdown is returned by [Coordinator.Subscribe] and other
	// operations after [Coordinator.Close] has been called.
	ErrCoordinatorShutdown = errors.New("health: coordinator has been shut down")

	// ErrInvalidServiceName is returned when a service name is empty or invalid.
	ErrInvalidServiceName = errors.New("health: invalid service name")

	// ErrNilCheckFunc is returned when a nil [Checker] function is provided.
	ErrNilCheckFunc = errors.New("health: check function cannot be nil")

	// ErrCheckTimeout is returned when a health check exceeds the configured
	// timeout ([WithCheckTimeout]).
	ErrCheckTimeout = errors.New("health: check timeout")

	// ErrSchedulerManaged is returned by [Coordinator.RunHealthCheckCycle] when
	// the cycle is managed by a scheduler via [Coordinator.RegisterHealthCheckSchedulerFunc].
	// Direct calls are not allowed in this mode.
	ErrSchedulerManaged = errors.New("health: function is managed by scheduler, direct calls not allowed")
)

// WrapCheckError wraps a health check error with service context.
// Uses core/errors for consistent error wrapping.
func WrapCheckError(err error, service string) error {
	return coreerrs.WrapOperationWithContext(err, "health check", service)
}

// WrapWatchError wraps a watch error with service context.
func WrapWatchError(err error, service string) error {
	return coreerrs.WrapOperationWithContext(err, "watch service", service)
}

// WrapShutdownError wraps a shutdown error with context.
func WrapShutdownError(err error) error {
	return coreerrs.WrapOperation(err, "shutdown coordinator")
}
