// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"
)

func BenchmarkNew(b *testing.B) {
	for b.Loop() {
		New()
	}
}

func BenchmarkNewHTTPClient(b *testing.B) {
	for b.Loop() {
		NewHTTPClient()
	}
}

// BenchmarkHTTPClient_PostBody measures the retry round-tripper cost for
// body-bearing requests: with GetBody available the body must not be
// buffered, and with retries disabled buffering is unnecessary entirely.
func BenchmarkHTTPClient_PostBody(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	payload := bytes.Repeat([]byte("x"), 4096)
	ctx := b.Context()

	b.Run("with_getbody_retries_enabled", func(b *testing.B) {
		c := New(WithSSRFAllowedCIDRs(netip.MustParsePrefix("127.0.0.0/8")), WithRetryMax(2))
		b.ReportAllocs()
		for b.Loop() {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL, bytes.NewReader(payload))
			if err != nil {
				b.Fatal(err)
			}
			resp, err := c.Do(req)
			if err != nil {
				b.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	})

	b.Run("opaque_body_no_retries", func(b *testing.B) {
		c := New(WithSSRFAllowedCIDRs(netip.MustParsePrefix("127.0.0.0/8")), WithRetryMax(0))
		b.ReportAllocs()
		for b.Loop() {
			// Hide the reader type so net/http cannot derive GetBody.
			body := struct{ io.Reader }{bytes.NewReader(payload)}
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL, body)
			if err != nil {
				b.Fatal(err)
			}
			resp, err := c.Do(req)
			if err != nil {
				b.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	})
}

func BenchmarkHTTPClient_Get(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTPClient(WithSSRFAllowedCIDRs(netip.MustParsePrefix("127.0.0.0/8")), WithRetryMax(0))
	ctx := b.Context()

	for b.Loop() {
		resp, err := c.Get(ctx, srv.URL)
		if err != nil {
			b.Fatalf("Get() error = %v", err)
		}
		resp.Body.Close()
	}
}

// BenchmarkHTTPClient_RetryAfter503 measures an HTTP/1.1 request that first
// gets a small 503 and succeeds on the retry. conns/op reports how many new
// TCP connections the client opened per request: draining the discarded 503
// lets the retry reuse the same connection.
func BenchmarkHTTPClient_RetryAfter503(b *testing.B) {
	var calls, conns atomic.Int64
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1)%2 == 1 {
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	srv.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		if s == http.StateNew {
			conns.Add(1)
		}
	}
	srv.Start()
	defer srv.Close()

	c := New(WithSSRFAllowedCIDRs(netip.MustParsePrefix("127.0.0.0/8")), WithRetryMax(1), WithRetryWait(time.Nanosecond, time.Nanosecond))
	// Discarded bodies are drained only under a deadline.
	ctx, cancel := context.WithTimeout(b.Context(), time.Hour)
	defer cancel()

	b.ReportAllocs()
	for b.Loop() {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
		if err != nil {
			b.Fatal(err)
		}
		resp, err := c.Do(req)
		if err != nil {
			b.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	b.ReportMetric(float64(conns.Load())/float64(b.N), "conns/op")
}

// BenchmarkHTTPClientMetrics_Cached measures the warm metric-handle lookups
// performed on every RoundTrip; cache hits must not allocate.
func BenchmarkHTTPClientMetrics_Cached(b *testing.B) {
	m := newHTTPClientMetrics(nil, "")
	m.durationFor(http.MethodGet)
	m.totalFor(http.MethodGet, "2xx")
	m.errorsFor(http.MethodGet)

	b.Run("durationFor", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			m.durationFor(http.MethodGet)
		}
	})
	b.Run("totalFor", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			m.totalFor(http.MethodGet, "2xx")
		}
	})
	b.Run("errorsFor", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			m.errorsFor(http.MethodGet)
		}
	})
}
