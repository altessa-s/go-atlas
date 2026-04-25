// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// UseLogger sets the logger for the builder and all created components.
func (b *OutboxBuilder) UseLogger(v *slog.Logger) *OutboxBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *OutboxBuilder) UseDefaultLogger() *OutboxBuilder {
	return b.UseLogger(slog.Default())
}

// UseScheduler sets the task scheduler for background processes.
func (b *OutboxBuilder) UseScheduler(v corescheduler.TaskRegistrar) *OutboxBuilder {
	b.scheduler = v
	return b
}
