// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/altessa-s/go-atlas/observability/metrics/adapters"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
	observabilityconfig "github.com/altessa-s/go-atlas/config/observability"
	metricsfactory "github.com/altessa-s/go-atlas/factory/observability/metrics"
	promadapter "github.com/altessa-s/go-atlas/observability/metrics/adapters/prometheus"
)

// Builder returns the metrics adapter builder of the Prometheus type, ready
// for metricsfactory.CollectorBuilder.UseAdapter. The adapter registers in a
// private registry when adapters.prometheus.customRegistry is set and in the
// process-wide default registry otherwise.
func Builder() metricsfactory.AdapterBuilder {
	return func(cfg *observabilityconfig.MetricsAdapters) (adapters.Adapter, error) {
		custom := cfg != nil && cfg.Prometheus != nil && cfg.Prometheus.CustomRegistry
		opts := coreslices.AppendIfFunc(nil, custom, func() []promadapter.Option {
			return []promadapter.Option{promadapter.WithRegistry(prometheus.NewRegistry())}
		})
		return promadapter.New(opts...), nil
	}
}
