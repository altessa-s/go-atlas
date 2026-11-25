// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/altessa-s/go-atlas-core/types/ptr"
	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters/console"
	"github.com/altessa-s/go-atlas/observability/tracing/sampler"

	observabilityconfig "github.com/altessa-s/go-atlas/config/observability"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

// TracerBuilder assembles a [tracing.TracerProvider] from configuration
// using a fluent API with deferred error accumulation.
// Returns [tracing.Noop] when config is nil or tracing is disabled.
//
// Console and noop adapters are built in; register any other backend with
// [TracerBuilder.UseAdapter], for example the OTLP one from
// factory/observability/tracing/adapters/otlp.
type TracerBuilder struct {
	corefactory.Base
	meter    metrics.Meter
	cfg      *observabilityconfig.Tracing
	adapters map[observabilityconfig.TracingType]AdapterBuilder
	errs     []error
}

// AdapterBuilder creates the tracing adapter of one tracing type from the
// adapters configuration block (nil when the block is absent).
type AdapterBuilder func(ctx context.Context, cfg *observabilityconfig.TracingAdapters) (adapters.Adapter, error)

// New creates a new [TracerBuilder] for the given tracing config.
func New(cfg *observabilityconfig.Tracing) *TracerBuilder {
	return &TracerBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build returns the provider that owns the configured exporter.
// Returns [tracing.Noop] if config is nil or tracing is disabled.
func (b *TracerBuilder) Build(ctx context.Context) (tracing.TracerProvider, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil || !b.cfg.Enabled {
		return tracing.Noop(), nil
	}

	batch := &b.cfg.Batch
	if err := batch.Validate(); err != nil {
		return nil, b.WrapError(err, "batch export settings")
	}

	s, err := b.createSampler()
	if err != nil {
		return nil, b.WrapError(err, "create sampler")
	}

	adapter, err := b.createAdapter(ctx)
	if err != nil {
		return nil, b.WrapError(err, "create adapter")
	}

	opts := []tracing.Option{
		tracing.WithServiceName(b.cfg.ServiceName),
		tracing.WithServiceVersion(b.cfg.ServiceVersion),
		tracing.WithEnvironment(b.cfg.Environment),
		tracing.WithAdapter(adapter),
		tracing.WithSampler(s),
		tracing.WithMeter(b.meter),
		tracing.WithMaxQueueSize(ptr.Unwrap(batch.MaxQueueSize)),
		tracing.WithMaxExportBatchSize(ptr.Unwrap(batch.MaxExportBatchSize)),
		tracing.WithScheduledDelay(ptr.Unwrap(batch.ScheduledDelay)),
		tracing.WithExportTimeout(ptr.Unwrap(batch.ExportTimeout)),
	}

	return tracing.New(opts...), nil //nolint:contextcheck // The export worker lives until tracer shutdown.
}

// createAdapter creates the adapter of the configured type: console and noop
// are built in, every other type comes from a builder registered with
// UseAdapter.
func (b *TracerBuilder) createAdapter(ctx context.Context) (adapters.Adapter, error) {
	switch b.cfg.Type {
	case observabilityconfig.TracingTypeConsole:
		var consoleCfg *observabilityconfig.TracingConsole
		if b.cfg.Adapters != nil {
			consoleCfg = b.cfg.Adapters.Console
		}
		return createConsoleAdapter(consoleCfg), nil
	case observabilityconfig.TracingTypeNoop:
		return nil, nil //nolint:nilnil // Intentional: nil adapter signals noop mode
	case observabilityconfig.TracingTypeOTLP:
		// Registered with UseAdapter.
	}
	build, ok := b.adapters[b.cfg.Type]
	if !ok {
		if b.cfg.Type == observabilityconfig.TracingTypeOTLP {
			return nil, fmt.Errorf("tracing type %q is not registered: add factory/observability/tracing/adapters/otlp "+
				"with UseAdapter(%q, otlpfactory.Builder())", b.cfg.Type, b.cfg.Type)
		}
		return nil, fmt.Errorf("unknown tracing type: %s", b.cfg.Type)
	}
	return build(ctx, b.cfg.Adapters)
}

// createConsoleAdapter creates a console adapter from config.
func createConsoleAdapter(cfg *observabilityconfig.TracingConsole) adapters.Adapter {
	var opts []console.Option
	if cfg != nil {
		opts = append(opts, console.WithPrettyPrint(cfg.PrettyPrint), console.WithTimestamps(cfg.Timestamps))
	}
	return console.New(opts...)
}

// createSampler creates a sampler based on configuration.
func (b *TracerBuilder) createSampler() (sampler.Sampler, error) {
	cfg := b.cfg.Sampler
	if cfg == nil {
		return sampler.AlwaysOn(), nil
	}

	switch cfg.Type {
	case observabilityconfig.SamplerTypeAlwaysOn:
		return sampler.AlwaysOn(), nil
	case observabilityconfig.SamplerTypeAlwaysOff:
		return sampler.AlwaysOff(), nil
	case observabilityconfig.SamplerTypeTraceIDRatio:
		return sampler.NewTraceIDRatio(cfg.Ratio), nil
	case observabilityconfig.SamplerTypeParentBased:
		root := sampler.NewTraceIDRatio(cfg.Ratio)
		return sampler.NewParentBased(root), nil
	default:
		return nil, fmt.Errorf("unknown sampler type: %s", cfg.Type)
	}
}
