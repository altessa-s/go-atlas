// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"log/slog"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters/console"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters/otlp"
	"github.com/altessa-s/go-atlas/observability/tracing/sampler"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
)

// TracerBuilder assembles a [tracing.Tracer] from configuration
// using a fluent API with deferred error accumulation.
// Returns [tracing.Noop] when config is nil or tracing is disabled.
type TracerBuilder struct {
	corefactory.Base
	cfg  *config.Tracing
	errs []error
}

// New creates a new [TracerBuilder] for the given tracing config.
func New(cfg *config.Tracing) *TracerBuilder {
	return &TracerBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build assembles and returns the tracer.
// Returns [tracing.Noop] if config is nil or tracing is disabled.
func (b *TracerBuilder) Build(ctx context.Context) (tracing.Tracer, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil || !b.cfg.Enabled {
		return tracing.Noop(), nil
	}

	adapter, err := b.createAdapter(ctx)
	if err != nil {
		return nil, b.WrapError(err, "create adapter")
	}

	s, err := b.createSampler()
	if err != nil {
		return nil, b.WrapError(err, "create sampler")
	}

	opts := []tracing.Option{
		tracing.WithServiceName(b.cfg.ServiceName),
		tracing.WithServiceVersion(b.cfg.ServiceVersion),
		tracing.WithEnvironment(b.cfg.Environment),
		tracing.WithAdapter(adapter),
		tracing.WithSampler(s),
	}

	return tracing.New(opts...), nil
}

// createAdapter creates an adapter based on configuration.
func (b *TracerBuilder) createAdapter(ctx context.Context) (adapters.Adapter, error) {
	switch b.cfg.Type {
	case config.TracingTypeOTLP:
		return b.createOTLPAdapter(ctx)
	case config.TracingTypeConsole:
		var consoleCfg *config.TracingConsole
		if b.cfg.Adapters != nil {
			consoleCfg = b.cfg.Adapters.Console
		}
		return createConsoleAdapter(consoleCfg), nil
	case config.TracingTypeNoop:
		return nil, nil //nolint:nilnil // Intentional: nil adapter signals noop mode
	default:
		return nil, b.Errorf("unknown tracing type: %s", b.cfg.Type)
	}
}

// createOTLPAdapter creates an OTLP adapter from config.
func (b *TracerBuilder) createOTLPAdapter(ctx context.Context) (adapters.Adapter, error) {
	var cfg *config.TracingOTLP
	if b.cfg.Adapters != nil {
		cfg = b.cfg.Adapters.OTLP
	}
	if cfg == nil {
		cfg = &config.TracingOTLP{
			Endpoint: "localhost:4317",
			Protocol: config.OTLPProtocolGRPC,
		}
	}

	opts := []otlp.Option{otlp.WithEndpoint(cfg.Endpoint)}
	opts = slices.AppendIf(opts, cfg.Insecure, otlp.WithInsecure())
	opts = slices.AppendIf(opts, cfg.Compression, otlp.WithCompression())
	opts = slices.AppendIf(opts, len(cfg.Headers) > 0, otlp.WithHeaders(cfg.Headers))

	proxyOpts, err := cfg.Proxy.ClientOptions()
	if err != nil {
		return nil, b.WrapError(err, "failed to materialize OTLP proxy options")
	}
	if len(proxyOpts) > 0 {
		opts = append(opts, otlp.WithGRPCClientOptions(proxyOpts...))
	}

	return otlp.New(ctx, opts...)
}

// createConsoleAdapter creates a console adapter from config.
func createConsoleAdapter(cfg *config.TracingConsole) adapters.Adapter {
	var opts []console.Option
	if cfg != nil {
		opts = slices.AppendIf(opts, cfg.PrettyPrint, console.WithPrettyPrint())
		opts = slices.AppendIf(opts, cfg.Timestamps, console.WithTimestamps())
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
	case config.SamplerTypeAlwaysOn:
		return sampler.AlwaysOn(), nil
	case config.SamplerTypeAlwaysOff:
		return sampler.AlwaysOff(), nil
	case config.SamplerTypeTraceIDRatio:
		return sampler.NewTraceIDRatio(cfg.Ratio), nil
	case config.SamplerTypeParentBased:
		root := sampler.NewTraceIDRatio(cfg.Ratio)
		return sampler.NewParentBased(root), nil
	default:
		return nil, b.Errorf("unknown sampler type: %s", cfg.Type)
	}
}
