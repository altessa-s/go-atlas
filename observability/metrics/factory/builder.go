// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/metrics/adapters"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	promadapter "github.com/altessa-s/go-atlas/observability/metrics/adapters/prometheus"
)

// CollectorBuilder assembles a [metrics.Collector] from configuration
// using a fluent API with deferred error accumulation.
// Returns [metrics.Noop] when config is nil or metrics are disabled.
type CollectorBuilder struct {
	corefactory.Base
	cfg  *config.Metrics
	errs []error
}

// New creates a new [CollectorBuilder] for the given metrics config.
func New(cfg *config.Metrics) *CollectorBuilder {
	return &CollectorBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build assembles and returns the metrics collector.
// Returns [metrics.Noop] if config is nil or metrics are disabled.
func (b *CollectorBuilder) Build() (metrics.Collector, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil || !b.cfg.Enabled {
		return metrics.Noop(), nil
	}

	adapter, err := b.createAdapter()
	if err != nil {
		return nil, b.WrapError(err, "create adapter")
	}

	opts := []metrics.Option{
		metrics.WithServiceName(b.cfg.ServiceName),
		metrics.WithAdapter(adapter),
	}

	return metrics.New(opts...), nil
}

// createAdapter creates an adapter based on configuration.
func (b *CollectorBuilder) createAdapter() (adapters.Adapter, error) {
	var adaptersCfg *config.MetricsAdapters
	if b.cfg.Adapters != nil {
		adaptersCfg = b.cfg.Adapters
	}

	switch b.cfg.Type {
	case config.MetricsTypePrometheus:
		var promCfg *config.MetricsPrometheus
		if adaptersCfg != nil {
			promCfg = adaptersCfg.Prometheus
		}
		return createPrometheusAdapter(promCfg), nil
	case config.MetricsTypeNoop:
		return nil, nil //nolint:nilnil // Intentional: nil adapter signals noop mode
	default:
		return nil, b.Errorf("unknown metrics type: %s", b.cfg.Type)
	}
}

// createPrometheusAdapter creates a Prometheus adapter from config.
func createPrometheusAdapter(cfg *config.MetricsPrometheus) *promadapter.Adapter {
	var opts []promadapter.Option

	if cfg != nil && cfg.CustomRegistry {
		registry := prometheus.NewRegistry()
		opts = append(opts, promadapter.WithRegistry(registry))
	}

	return promadapter.New(opts...)
}
