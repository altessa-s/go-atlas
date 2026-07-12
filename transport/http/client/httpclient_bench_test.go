// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
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
		c := New(WithRetryMax(2))
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
		c := New(WithRetryMax(0))
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

	c := NewHTTPClient(WithRetryMax(0))
	ctx := b.Context()

	for b.Loop() {
		resp, err := c.Get(ctx, srv.URL)
		if err != nil {
			b.Fatalf("Get() error = %v", err)
		}
		resp.Body.Close()
	}
}
