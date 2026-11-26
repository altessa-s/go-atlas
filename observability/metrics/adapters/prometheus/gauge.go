// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package prometheus

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/altessa-s/go-atlas/observability/metrics/adapters"
)

// gaugeVec creates children whose value is shared across all collectors using
// the registry. A mutex protects each value without a float CAS loop.
type gaugeVec struct {
	*prometheus.MetricVec
}

func newGaugeVec(d *adapters.Desc) *gaugeVec {
	desc := prometheus.NewDesc(d.Name, d.Help, d.LabelNames, nil)
	return &gaugeVec{MetricVec: prometheus.NewMetricVec(desc, func(values ...string) prometheus.Metric {
		g := &gauge{desc: desc}
		labels := make(prometheus.Labels, len(values))
		for i, value := range values {
			labels[d.LabelNames[i]] = value
		}
		g.Metric = prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name:        d.Name,
			Help:        d.Help,
			ConstLabels: labels,
		}, g.load)
		return g
	})}
}

type gauge struct {
	prometheus.Metric
	desc *prometheus.Desc

	mu    sync.Mutex
	value float64
}

// Desc keeps the vector descriptor. The embedded metric serializes the
// resolved label values and calls load when Prometheus scrapes the child.
func (g *gauge) Desc() *prometheus.Desc { return g.desc }

func (g *gauge) load() float64 {
	g.mu.Lock()
	value := g.value
	g.mu.Unlock()
	return value
}

func (g *gauge) Add(delta float64) {
	g.mu.Lock()
	g.value += delta
	g.mu.Unlock()
}

func (g *gauge) Set(value float64) {
	g.mu.Lock()
	g.value = value
	g.mu.Unlock()
}
