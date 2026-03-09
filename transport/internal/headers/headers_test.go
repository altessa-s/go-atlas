// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package headers

import (
	"net/http"
	"testing"

	"google.golang.org/grpc/metadata"
)

func TestHTTPHeaderGetter_GetHeader(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.Header.Add("X-Forwarded-For", "5.6.7.8")

	g := NewHTTPHeaderGetter(req)
	vals := g.GetHeader("X-Forwarded-For")
	if len(vals) != 2 {
		t.Fatalf("GetHeader() returned %d values, want 2", len(vals))
	}
}

func TestHTTPHeaderGetter_GetSingleHeader(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Content-Type", "application/json")

	g := NewHTTPHeaderGetter(req)
	if got := g.GetSingleHeader("Content-Type"); got != "application/json" {
		t.Fatalf("GetSingleHeader() = %q, want %q", got, "application/json")
	}
}

func TestHTTPHeaderGetter_NilRequest(t *testing.T) {
	g := NewHTTPHeaderGetter(nil)
	if g != nil {
		t.Fatal("NewHTTPHeaderGetter(nil) should return nil")
	}
}

func TestHTTPHeaderGetter_NilReceiver(t *testing.T) {
	var g *HTTPHeaderGetter
	if vals := g.GetHeader("X-Test"); vals != nil {
		t.Fatalf("nil receiver GetHeader() = %v, want nil", vals)
	}
	if got := g.GetSingleHeader("X-Test"); got != "" {
		t.Fatalf("nil receiver GetSingleHeader() = %q, want empty", got)
	}
}

func TestHTTPHeaderGetter_MissingHeader(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	g := NewHTTPHeaderGetter(req)

	if vals := g.GetHeader("X-Missing"); len(vals) != 0 {
		t.Fatalf("GetHeader(missing) = %v, want empty", vals)
	}
	if got := g.GetSingleHeader("X-Missing"); got != "" {
		t.Fatalf("GetSingleHeader(missing) = %q, want empty", got)
	}
}

func TestGRPCHeaderGetter_GetHeader(t *testing.T) {
	md := metadata.Pairs("x-forwarded-for", "1.2.3.4", "x-forwarded-for", "5.6.7.8")
	ctx := metadata.NewIncomingContext(t.Context(), md)

	g := NewGRPCHeaderGetter(ctx)
	vals := g.GetHeader("x-forwarded-for")
	if len(vals) != 2 {
		t.Fatalf("GetHeader() returned %d values, want 2", len(vals))
	}
}

func TestGRPCHeaderGetter_GetSingleHeader(t *testing.T) {
	md := metadata.Pairs("request-id", "abc123")
	ctx := metadata.NewIncomingContext(t.Context(), md)

	g := NewGRPCHeaderGetter(ctx)
	if got := g.GetSingleHeader("request-id"); got != "abc123" {
		t.Fatalf("GetSingleHeader() = %q, want %q", got, "abc123")
	}
}

func TestGRPCHeaderGetter_NilReceiver(t *testing.T) {
	var g *GRPCHeaderGetter
	if vals := g.GetHeader("x-test"); vals != nil {
		t.Fatalf("nil receiver GetHeader() = %v, want nil", vals)
	}
	if got := g.GetSingleHeader("x-test"); got != "" {
		t.Fatalf("nil receiver GetSingleHeader() = %q, want empty", got)
	}
}

func TestGRPCHeaderGetter_NilContext(t *testing.T) {
	g := &GRPCHeaderGetter{ctx: nil}
	if vals := g.GetHeader("x-test"); vals != nil {
		t.Fatalf("nil ctx GetHeader() = %v, want nil", vals)
	}
}

func TestGRPCHeaderGetter_MissingHeader(t *testing.T) {
	ctx := metadata.NewIncomingContext(t.Context(), metadata.New(nil))
	g := NewGRPCHeaderGetter(ctx)

	if got := g.GetSingleHeader("x-missing"); got != "" {
		t.Fatalf("GetSingleHeader(missing) = %q, want empty", got)
	}
}
