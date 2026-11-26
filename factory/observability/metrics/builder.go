// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"
	"net/http"

	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/metrics/adapters"

	observabilityconfig "github.com/altessa-s/go-atlas/config/observability"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

// AdapterBuilder creates the metrics adapter of one metrics type from the
// adapters configuration block (nil when the block is absent).
type AdapterBuilder func(cfg *observabilityconfig.MetricsAdapters) (adapters.Adapter, error)

// CollectorBuilder assembles a [metrics.Collector] from configuration
// using a fluent API with deferred error accumulation.
// Returns [metrics.Noop] when config is nil or metrics are disabled.
//
// The builder knows no concrete adapter: register one per metrics type with
// [CollectorBuilder.UseAdapter], for example the Prometheus one from
// factory/observability/metrics/adapters/prometheus.
type CollectorBuilder struct {
	corefactory.Base
	cfg      *observabilityconfig.Metrics
	adapters map[observabilityconfig.MetricsType]AdapterBuilder
	errs     []error
}

// New creates a new [CollectorBuilder] for the given metrics config.
func New(cfg *observabilityconfig.Metrics) *CollectorBuilder {
	return &CollectorBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build assembles and returns the metrics collector.
// Returns [metrics.Noop] if config is nil or metrics are disabled.
func (b *CollectorBuilder) Build() (metrics.Collector, error) {
	collector, _, err := b.build()
	return collector, err
}

// BuildWithHandler assembles the collector and returns the scrape handler of
// its adapter. The handler is nil when the adapter doesn't serve metrics over HTTP.
func (b *CollectorBuilder) BuildWithHandler() (metrics.Collector, http.Handler, error) {
	collector, adapter, err := b.build()
	if err != nil {
		return nil, nil, err
	}
	if httpAdapter, ok := adapter.(adapters.HTTPHandler); ok {
		return collector, httpAdapter.Handler(), nil
	}
	return collector, nil, nil
}

func (b *CollectorBuilder) build() (metrics.Collector, adapters.Adapter, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, nil, err
	}

	if b.cfg == nil || !b.cfg.Enabled {
		return metrics.Noop(), nil, nil
	}

	adapter, err := b.createAdapter()
	if err != nil {
		return nil, nil, b.WrapError(err, "create adapter")
	}

	opts := []metrics.Option{
		metrics.WithServiceName(b.cfg.ServiceName),
		metrics.WithAdapter(adapter),
		metrics.WithLogger(b.Logger()),
	}

	return metrics.New(opts...), adapter, nil
}

// createAdapter creates the adapter registered for the configured type.
func (b *CollectorBuilder) createAdapter() (adapters.Adapter, error) {
	if b.cfg.Type == observabilityconfig.MetricsTypeNoop {
		return nil, nil //nolint:nilnil // Intentional: nil adapter signals noop mode
	}
	build, ok := b.adapters[b.cfg.Type]
	if !ok {
		if b.cfg.Type == observabilityconfig.MetricsTypePrometheus {
			return nil, corefactory.Errorf(corefactory.ErrNotRegistered, "metrics type %q is not registered: add factory/observability/metrics/adapters/prometheus "+
				"with UseAdapter(%q, promfactory.Builder())", b.cfg.Type, b.cfg.Type)
		}
		return nil, corefactory.Errorf(corefactory.ErrUnsupported, "unknown metrics type: %s", b.cfg.Type)
	}
	return build(b.cfg.Adapters)
}
