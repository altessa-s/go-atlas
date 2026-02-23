// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters/console"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters/otlp"
	"github.com/altessa-s/go-atlas/observability/tracing/sampler"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
)

// Factory creates [tracing.Tracer] instances from [config.Tracing] configuration.
// Safe for concurrent use after construction.
type Factory struct {
	corefactory.Base
}

// New creates a new Factory with the given options.
func New(opts ...Option) *Factory {
	cfg := newOptions(opts...)
	return &Factory{
		Base: corefactory.NewBase(cfg.logger),
	}
}

// CreateFromConfig creates a [tracing.Tracer] from [config.Tracing].
// Returns [tracing.Noop] if config is nil or tracing is disabled.
func (f *Factory) CreateFromConfig(ctx context.Context, cfg *config.Tracing, opts ...tracing.Option) (tracing.Tracer, error) {
	if cfg == nil || !cfg.Enable {
		return tracing.Noop(), nil
	}

	adapter, err := f.createAdapter(ctx, cfg)
	if err != nil {
		return nil, f.WrapError(err, "create adapter")
	}

	s, err := f.createSampler(cfg.Sampler)
	if err != nil {
		return nil, f.WrapError(err, "create sampler")
	}

	allOpts := []tracing.Option{
		tracing.WithServiceName(cfg.ServiceName),
		tracing.WithServiceVersion(cfg.ServiceVersion),
		tracing.WithEnvironment(cfg.Environment),
		tracing.WithAdapter(adapter),
		tracing.WithSampler(s),
	}
	allOpts = append(allOpts, opts...)

	return tracing.New(allOpts...), nil
}

// CreateNoop returns a no-op [tracing.Tracer] via [tracing.Noop].
func (f *Factory) CreateNoop() tracing.Tracer {
	return tracing.Noop()
}

// createAdapter creates an adapter based on configuration.
func (f *Factory) createAdapter(ctx context.Context, cfg *config.Tracing) (adapters.Adapter, error) {
	var adaptersCfg *config.TracingAdapters
	if cfg.Adapters != nil {
		adaptersCfg = cfg.Adapters
	}

	switch cfg.Type {
	case config.TracingTypeOTLP:
		var otlpCfg *config.TracingOTLP
		if adaptersCfg != nil {
			otlpCfg = adaptersCfg.OTLP
		}
		return f.createOTLPAdapterFromConfig(ctx, otlpCfg)
	case config.TracingTypeConsole:
		var consoleCfg *config.TracingConsole
		if adaptersCfg != nil {
			consoleCfg = adaptersCfg.Console
		}
		return f.createConsoleAdapterFromConfig(consoleCfg), nil
	case config.TracingTypeNoop:
		return nil, nil //nolint:nilnil // Intentional: nil adapter signals noop mode
	default:
		return nil, f.Errorf("unknown tracing type: %s", cfg.Type)
	}
}

// createOTLPAdapterFromConfig creates an OTLP adapter from config.
func (f *Factory) createOTLPAdapterFromConfig(ctx context.Context, cfg *config.TracingOTLP) (adapters.Adapter, error) {
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

	return otlp.New(ctx, opts...)
}

// createConsoleAdapterFromConfig creates a console adapter from config.
func (f *Factory) createConsoleAdapterFromConfig(cfg *config.TracingConsole) adapters.Adapter {
	var opts []console.Option
	if cfg != nil {
		opts = slices.AppendIf(opts, cfg.PrettyPrint, console.WithPrettyPrint())
		opts = slices.AppendIf(opts, cfg.Timestamps, console.WithTimestamps())
	}
	return console.New(opts...)
}

// createSampler creates a sampler based on configuration.
func (f *Factory) createSampler(cfg *config.TracingSampler) (sampler.Sampler, error) {
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
		return nil, f.Errorf("unknown sampler type: %s", cfg.Type)
	}
}
