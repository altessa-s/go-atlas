// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	observabilityconfig "github.com/altessa-s/go-atlas/config/observability"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

// UseLogger sets the logger for the builder and all created components.
func (b *CollectorBuilder) UseLogger(v *slog.Logger) *CollectorBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *CollectorBuilder) UseDefaultLogger() *CollectorBuilder {
	return b.UseLogger(slog.Default())
}

// UseAdapter registers the adapter builder of one metrics type
// (observabilityconfig.MetricsTypePrometheus, ...). A later registration of
// the same type replaces the earlier one.
func (b *CollectorBuilder) UseAdapter(typ observabilityconfig.MetricsType, build AdapterBuilder) *CollectorBuilder {
	if build == nil {
		b.errs = append(b.errs, corefactory.Errorf(coreerrs.ErrInvalidArgument, "UseAdapter %q: nil builder", typ))
		return b
	}
	if b.adapters == nil {
		b.adapters = map[observabilityconfig.MetricsType]AdapterBuilder{}
	}
	b.adapters[typ] = build
	return b
}
