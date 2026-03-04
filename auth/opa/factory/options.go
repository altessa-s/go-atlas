// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	"github.com/altessa-s/go-atlas/observability/health"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// UseLogger sets the logger for the builder and all created components.
func (b *ManagerBuilder) UseLogger(v *slog.Logger) *ManagerBuilder {
	if v != nil {
		b.Base = corefactory.NewBase(v)
	}
	return b
}

// UseScheduler sets the task registrar used for periodic policy update cycles.
func (b *ManagerBuilder) UseScheduler(v corescheduler.TaskRegistrar) *ManagerBuilder {
	b.scheduler = v
	return b
}

// UseHealthCoordinator sets the health coordinator for registering the
// created manager as a health checker.
func (b *ManagerBuilder) UseHealthCoordinator(v *health.Coordinator) *ManagerBuilder {
	b.healthCoordinator = v
	return b
}
