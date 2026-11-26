// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"io"
	"log/slog"

	"github.com/altessa-s/go-atlas/observability/metrics"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	observabilityconfig "github.com/altessa-s/go-atlas/config/observability"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

// UseLogger sets the logger for the builder and all created components.
func (b *TracerBuilder) UseLogger(v *slog.Logger) *TracerBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *TracerBuilder) UseDefaultLogger() *TracerBuilder {
	return b.UseLogger(slog.Default())
}

// UseAdapter registers the adapter builder of one tracing type
// (observabilityconfig.TracingTypeOTLP, ...). A later registration of the
// same type replaces the earlier one.
func (b *TracerBuilder) UseAdapter(typ observabilityconfig.TracingType, build AdapterBuilder) *TracerBuilder {
	if build == nil {
		b.errs = append(b.errs, corefactory.Errorf(coreerrs.ErrInvalidArgument, "UseAdapter %q: nil builder", typ))
		return b
	}
	if b.adapters == nil {
		b.adapters = map[observabilityconfig.TracingType]AdapterBuilder{}
	}
	b.adapters[typ] = build
	return b
}

// UseMeter records batch exporter metrics through the supplied meter.
func (b *TracerBuilder) UseMeter(m metrics.Meter) *TracerBuilder {
	b.meter = m
	return b
}

// UseConsoleWriter sets the built-in console adapter's output. A nil writer uses os.Stdout.
func (b *TracerBuilder) UseConsoleWriter(w io.Writer) *TracerBuilder {
	b.consoleWriter = w
	return b
}
