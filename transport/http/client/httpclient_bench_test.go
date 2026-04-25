// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
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
