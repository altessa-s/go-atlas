// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"io/fs"
	"log/slog"

	"github.com/altessa-s/go-atlas/observability/health"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// UseLogger sets the logger for the builder and all created components.
func (b *ManagerBuilder) UseLogger(v *slog.Logger) *ManagerBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *ManagerBuilder) UseDefaultLogger() *ManagerBuilder {
	return b.UseLogger(slog.Default())
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

// UseEmbedFS sets the fs.FS and directory for the embed policy source.
func (b *ManagerBuilder) UseEmbedFS(fsys fs.FS, dir string) *ManagerBuilder {
	b.embedFS = fsys
	b.embedDir = dir
	return b
}
