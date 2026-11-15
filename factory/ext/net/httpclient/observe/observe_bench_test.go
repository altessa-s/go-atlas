// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package observe

import (
	"net/http"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/observability/health"

	httpclient "github.com/altessa-s/go-atlas-net/http/client"
)

var benchEvent = httpclient.RequestEvent{
	Method:      http.MethodGet,
	Host:        "api.example.com",
	StatusClass: "2xx",
	Duration:    time.Millisecond,
}

// BenchmarkRequestDone measures the per-request event handling: cached
// metric handles and the retry-rate window. It must remain alloc-free.
func BenchmarkRequestDone(b *testing.B) {
	b.Run("noop meter, no reporter", func(b *testing.B) {
		o := New(nil, nil)
		o.Attach(&fakeBreakers{})
		b.ReportAllocs()
		for b.Loop() {
			o.RequestDone(benchEvent)
		}
	})
	b.Run("noop meter, reporter", func(b *testing.B) {
		coord := health.New()
		b.Cleanup(coord.Close)
		o := New(nil, coord)
		o.Attach(&fakeBreakers{})
		b.ReportAllocs()
		for b.Loop() {
			o.RequestDone(benchEvent)
		}
	})
}

// BenchmarkRecordRequest exercises the retry-rate sliding-window write path.
// It must remain alloc-free — it runs on every HTTP request.
func BenchmarkRecordRequest(b *testing.B) {
	coord := health.New()
	b.Cleanup(coord.Close)

	h := newClientHealth(*newOptions(WithHealthRetryWindow(60*time.Second), WithHealthRetryMinSamples(10)), coord)
	h.attach(&fakeBreakers{})

	b.ReportAllocs()
	for b.Loop() {
		h.recordRequest("api.example.com", false)
	}
}

// BenchmarkCheckHealth exercises the read path executed by the health
// scheduler (and gRPC Watch fanout).
func BenchmarkCheckHealth(b *testing.B) {
	coord := health.New()
	b.Cleanup(coord.Close)

	h := newClientHealth(*newOptions(), coord)
	h.attach(&fakeBreakers{})

	ctx := b.Context()
	b.ReportAllocs()
	for b.Loop() {
		_ = h.CheckHealth(ctx)
	}
}
