// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"io/fs"
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

// UseMigrations sets the forward migration directory and schema containing the ledger.
func (b *Builder) UseMigrations(source fs.FS, schema string) *Builder {
	b.source = source
	b.schema = schema
	return b
}
