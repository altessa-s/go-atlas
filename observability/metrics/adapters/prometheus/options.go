// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

package prometheus

import "github.com/prometheus/client_golang/prometheus"

// options contains configuration for the Prometheus adapter.
type options struct {
	// registerer is the Prometheus registerer to use.
	// If nil, prometheus.DefaultRegisterer is used.
	registerer prometheus.Registerer `optgen:"-"`

	// gatherer is the Prometheus gatherer to use.
	// If nil, prometheus.DefaultGatherer is used.
	gatherer prometheus.Gatherer `optgen:"-"`
}

// WithRegistry sets both the registerer and gatherer to the given registry.
func WithRegistry(registry *prometheus.Registry) Option {
	return func(o *options) {
		o.registerer = registry
		o.gatherer = registry
	}
}
