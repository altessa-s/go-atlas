// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/tracing"
)

// UseMeter supplies pool metric instruments.
func (b *Builder) UseMeter(m metrics.Meter) *Builder {
	b.meter = m
	return b
}

// UseTracer supplies the tracer for sampled PostgreSQL client spans.
func (b *Builder) UseTracer(t tracing.Tracer) *Builder {
	b.tracer = t
	return b
}

// UseLogger supplies the pool lifecycle logger.
func (b *Builder) UseLogger(l *slog.Logger) *Builder {
	b.SetLogger(l)
	return b
}

// UseDefaultLogger uses the process-wide logger for pool lifecycle messages.
func (b *Builder) UseDefaultLogger() *Builder {
	return b.UseLogger(slog.Default())
}

// UseHealthCoordinator registers each pool under postgres.<name> after startup.
func (b *Builder) UseHealthCoordinator(r health.Registrar) *Builder {
	b.health = r
	return b
}

// UseMigrator runs fn on the direct primary pool during startup.
// MigrationTimeout (YAML migrationTimeout, default 30s) bounds the hook unless the caller has an earlier deadline.
func (b *Builder) UseMigrator(fn func(context.Context, *sql.DB) error) *Builder {
	b.migrator = fn
	return b
}
