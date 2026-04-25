// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package shared

import (
	"context"
	"errors"
)

// Common lifecycle errors for observability components.
var (
	// ErrShutdown is returned when operations are performed on a shutdown component.
	ErrShutdown = errors.New("component has been shut down")

	// ErrAlreadyShutdown is returned when Shutdown is called on an already shutdown component.
	ErrAlreadyShutdown = errors.New("component is already shut down")
)

// Shutdownable is an interface for components that support graceful shutdown.
// Both TracerProvider and Collector implement this interface.
type Shutdownable interface {
	// Shutdown shuts down the component and releases resources.
	// After Shutdown is called, the component should not be used.
	// It should be safe to call Shutdown multiple times.
	Shutdown(ctx context.Context) error
}

// Flushable is an interface for components that can flush buffered data.
// Both TracerProvider and Collector implement this interface.
type Flushable interface {
	// ForceFlush forces an immediate export/flush of buffered data.
	// This is useful for ensuring data is exported before shutdown or at specific checkpoints.
	ForceFlush(ctx context.Context) error
}

// LifecycleManager combines Shutdownable and Flushable interfaces.
// This is the full lifecycle interface for observability providers.
type LifecycleManager interface {
	Shutdownable
	Flushable
}

// MultiShutdown shuts down multiple components, collecting all errors.
// Returns nil if all shutdowns succeed, otherwise returns a combined error.
func MultiShutdown(ctx context.Context, components ...Shutdownable) error {
	var errs []error
	for _, c := range components {
		if c == nil {
			continue
		}
		if err := c.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// MultiFlush flushes multiple components, collecting all errors.
// Returns nil if all flushes succeed, otherwise returns a combined error.
func MultiFlush(ctx context.Context, components ...Flushable) error {
	var errs []error
	for _, c := range components {
		if c == nil {
			continue
		}
		if err := c.ForceFlush(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
