// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/metrics/adapters"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	promadapter "github.com/altessa-s/go-atlas/observability/metrics/adapters/prometheus"
)

// Factory creates [metrics.Collector] instances from [config.Metrics] configuration.
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

// CreateFromConfig creates a [metrics.Collector] from [config.Metrics].
// Returns [metrics.Noop] if config is nil or metrics are disabled.
func (f *Factory) CreateFromConfig(cfg *config.Metrics, opts ...metrics.Option) (metrics.Collector, error) {
	if cfg == nil || !cfg.Enable {
		return metrics.Noop(), nil
	}

	adapter, err := f.createAdapter(cfg)
	if err != nil {
		return nil, f.WrapError(err, "create adapter")
	}

	allOpts := []metrics.Option{
		metrics.WithServiceName(cfg.ServiceName),
		metrics.WithAdapter(adapter),
	}
	allOpts = append(allOpts, opts...)

	return metrics.New(allOpts...), nil
}

// CreateNoop returns a no-op [metrics.Collector] via [metrics.Noop].
func (f *Factory) CreateNoop() metrics.Collector {
	return metrics.Noop()
}

// createAdapter creates an adapter based on configuration.
func (f *Factory) createAdapter(cfg *config.Metrics) (adapters.Adapter, error) {
	var adaptersCfg *config.MetricsAdapters
	if cfg.Adapters != nil {
		adaptersCfg = cfg.Adapters
	}

	switch cfg.Type {
	case config.MetricsTypePrometheus:
		var promCfg *config.MetricsPrometheus
		if adaptersCfg != nil {
			promCfg = adaptersCfg.Prometheus
		}
		return f.createPrometheusAdapter(promCfg), nil
	case config.MetricsTypeNoop:
		return nil, nil //nolint:nilnil // Intentional: nil adapter signals noop mode
	default:
		return nil, f.Errorf("unknown metrics type: %s", cfg.Type)
	}
}

// createPrometheusAdapter creates a Prometheus adapter from config.
func (f *Factory) createPrometheusAdapter(cfg *config.MetricsPrometheus) *promadapter.Adapter {
	var opts []promadapter.Option

	if cfg != nil && cfg.CustomRegistry {
		registry := prometheus.NewRegistry()
		opts = append(opts, promadapter.WithRegistry(registry))
	}

	return promadapter.New(opts...)
}
