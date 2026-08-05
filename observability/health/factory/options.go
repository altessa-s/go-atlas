// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// UseLogger sets the logger for the builder and all created components.
func (b *CoordinatorBuilder) UseLogger(v *slog.Logger) *CoordinatorBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *CoordinatorBuilder) UseDefaultLogger() *CoordinatorBuilder {
	return b.UseLogger(slog.Default())
}

// UseScheduler sets the task scheduler that drives the periodic health check
// cycle. Without it the coordinator answers queries but never re-checks watched
// services on its own, and `health.healthCheckInterval` has no effect.
func (b *CoordinatorBuilder) UseScheduler(v corescheduler.TaskRegistrar) *CoordinatorBuilder {
	b.scheduler = v
	return b
}
