// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/altessa-s/go-atlas/observability/metrics"

	prometheusadapter "github.com/altessa-s/go-atlas/observability/metrics/adapters/prometheus"
)

// BenchmarkValidateToken compares a full signature verification with a
// signature-cache hit; both run the complete claim policy and revocation step.
func BenchmarkValidateToken(b *testing.B) {
	idp := newTestIdP(b)
	raw := idp.sign(b, idp.claims(nil))

	b.Run("no-cache", func(b *testing.B) {
		p := idp.newProvider(b)
		for b.Loop() {
			if _, err := p.ValidateToken(b.Context(), raw); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("cache-hit", func(b *testing.B) {
		p := idp.newProvider(b, WithTokenCache(newMemCacher()))
		if _, err := p.ValidateToken(b.Context(), raw); err != nil {
			b.Fatal(err)
		}
		for b.Loop() {
			if _, err := p.ValidateToken(b.Context(), raw); err != nil {
				b.Fatal(err)
			}
		}
	})

	// The no-op collector hides the cost of label binding; a real Prometheus
	// registry exposes it.
	b.Run("cache-hit-prometheus", func(b *testing.B) {
		p := idp.newProvider(b, WithTokenCache(newMemCacher()), WithCollector(newPrometheusCollector()))
		if _, err := p.ValidateToken(b.Context(), raw); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		for b.Loop() {
			if _, err := p.ValidateToken(b.Context(), raw); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkObserveValidation isolates the per-validation metric bookkeeping
// from the validation itself.
func BenchmarkObserveValidation(b *testing.B) {
	idp := newTestIdP(b)
	validate := func() (map[string]any, error) { return nil, nil }

	for _, tc := range []struct {
		name string
		opts []Option
	}{
		{name: "noop"},
		{name: "prometheus", opts: []Option{WithCollector(newPrometheusCollector())}},
	} {
		b.Run(tc.name, func(b *testing.B) {
			p := idp.newProvider(b, tc.opts...)
			b.ReportAllocs()
			for b.Loop() {
				_, _ = p.observeValidation(validate)
			}
		})

		b.Run(tc.name+"-parallel", func(b *testing.B) {
			p := idp.newProvider(b, tc.opts...)
			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_, _ = p.observeValidation(validate)
				}
			})
		})
	}
}

// newPrometheusCollector returns a collector backed by a fresh Prometheus
// registry.
func newPrometheusCollector() metrics.Collector {
	return metrics.New(
		metrics.WithServiceName("bench"),
		metrics.WithAdapter(prometheusadapter.New(prometheusadapter.WithRegisterer(prometheus.NewRegistry()))),
	)
}
