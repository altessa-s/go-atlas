// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package headers

import (
	"net/http"
	"testing"

	"google.golang.org/grpc/metadata"
)

func BenchmarkHTTPHeaderGetter_GetSingleHeader(b *testing.B) {
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "abc123")
	g := NewHTTPHeaderGetter(req)

	for b.Loop() {
		g.GetSingleHeader("X-Request-ID")
	}
}

func BenchmarkHTTPHeaderGetter_GetHeader(b *testing.B) {
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.Header.Add("X-Forwarded-For", "1.2.3.4")
	req.Header.Add("X-Forwarded-For", "5.6.7.8")
	g := NewHTTPHeaderGetter(req)

	for b.Loop() {
		g.GetHeader("X-Forwarded-For")
	}
}

func BenchmarkGRPCHeaderGetter_GetSingleHeader(b *testing.B) {
	md := metadata.Pairs("request-id", "abc123")
	ctx := metadata.NewIncomingContext(b.Context(), md)
	g := NewGRPCHeaderGetter(ctx)

	for b.Loop() {
		g.GetSingleHeader("request-id")
	}
}
