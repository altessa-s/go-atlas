// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/observability/health"
)

// BenchmarkRecordRequest exercises the retry-rate sliding-window write path.
// It must remain alloc-free — the round-tripper invokes it on every HTTP
// request, including when health integration is disabled.
func BenchmarkRecordRequest(b *testing.B) {
	coord := health.New()
	b.Cleanup(coord.Close)

	opts := newOptions(WithHealthRetryWindow(60*time.Second), WithHealthRetryMinSamples(10))
	opts.healthCoordinator = coord
	cb := newCircuitBreakerClient(*opts, newHTTPClientMetrics(nil, ""))
	h := newHTTPClientHealth(*opts, cb)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		h.recordRequest("api.example.com", false)
	}
}

// BenchmarkRecordRequestNoCoordinator measures the disabled hot-path cost so
// regressions to the noop guard surface immediately.
func BenchmarkRecordRequestNoCoordinator(b *testing.B) {
	opts := newOptions()
	cb := newCircuitBreakerClient(*opts, newHTTPClientMetrics(nil, ""))
	h := newHTTPClientHealth(*opts, cb)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		h.recordRequest("api.example.com", true)
	}
}

// BenchmarkCheckHealth exercises the read path executed by the health
// scheduler (and gRPC Watch fanout).
func BenchmarkCheckHealth(b *testing.B) {
	coord := health.New()
	b.Cleanup(coord.Close)

	opts := newOptions()
	opts.healthCoordinator = coord
	cb := newCircuitBreakerClient(*opts, newHTTPClientMetrics(nil, ""))
	h := newHTTPClientHealth(*opts, cb)
	cb.health.Store(h)

	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = h.CheckHealth(ctx)
	}
}
