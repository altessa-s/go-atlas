// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package headers

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc/metadata"
)

func TestHTTPHeaderGetter_GetHeader(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.Header.Add("X-Forwarded-For", "5.6.7.8")

	g := NewHTTPHeaderGetter(req)
	vals := g.GetHeader("X-Forwarded-For")
	require.Len(t, vals, 2)
}

func TestHTTPHeaderGetter_GetSingleHeader(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Content-Type", "application/json")

	g := NewHTTPHeaderGetter(req)
	got := g.GetSingleHeader("Content-Type")
	require.Equal(t, "application/json", got)
}

func TestHTTPHeaderGetter_NilRequest(t *testing.T) {
	g := NewHTTPHeaderGetter(nil)
	require.Nil(t, g)
}

func TestHTTPHeaderGetter_NilReceiver(t *testing.T) {
	var g *HTTPHeaderGetter
	vals := g.GetHeader("X-Test")
	require.Nil(t, vals)
	got := g.GetSingleHeader("X-Test")
	require.Equal(t, "", got)
}

func TestHTTPHeaderGetter_MissingHeader(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	g := NewHTTPHeaderGetter(req)

	vals := g.GetHeader("X-Missing")
	require.Equal(t, 0, len(vals))
	got := g.GetSingleHeader("X-Missing")
	require.Equal(t, "", got)
}

func TestGRPCHeaderGetter_GetHeader(t *testing.T) {
	md := metadata.Pairs("x-forwarded-for", "1.2.3.4", "x-forwarded-for", "5.6.7.8")
	ctx := metadata.NewIncomingContext(t.Context(), md)

	g := NewGRPCHeaderGetter(ctx)
	vals := g.GetHeader("x-forwarded-for")
	require.Len(t, vals, 2)
}

func TestGRPCHeaderGetter_GetSingleHeader(t *testing.T) {
	md := metadata.Pairs("request-id", "abc123")
	ctx := metadata.NewIncomingContext(t.Context(), md)

	g := NewGRPCHeaderGetter(ctx)
	got := g.GetSingleHeader("request-id")
	require.Equal(t, "abc123", got)
}

func TestGRPCHeaderGetter_NilReceiver(t *testing.T) {
	var g *GRPCHeaderGetter
	vals := g.GetHeader("x-test")
	require.Nil(t, vals)
	got := g.GetSingleHeader("x-test")
	require.Equal(t, "", got)
}

func TestGRPCHeaderGetter_NilContext(t *testing.T) {
	g := &GRPCHeaderGetter{ctx: nil}
	vals := g.GetHeader("x-test")
	require.Nil(t, vals)
}

func TestGRPCHeaderGetter_MissingHeader(t *testing.T) {
	ctx := metadata.NewIncomingContext(t.Context(), metadata.New(nil))
	g := NewGRPCHeaderGetter(ctx)

	got := g.GetSingleHeader("x-missing")
	require.Equal(t, "", got)
}
