// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	grpcmetadata "google.golang.org/grpc/metadata"
)

func TestStaticTokenProvider(t *testing.T) {
	p := StaticTokenProvider("my-token")
	tok, err := p.Token(t.Context())
	require.NoError(t, err)
	require.Equal(t, "my-token", tok)
}

func TestTokenProviderFunc(t *testing.T) {
	f := TokenProviderFunc(func(ctx context.Context) (string, error) {
		return "dynamic", nil
	})
	tok, err := f.Token(t.Context())
	require.NoError(t, err)
	require.Equal(t, "dynamic", tok)
}

func TestClientInterceptor_Name(t *testing.T) {
	ic := ClientInterceptor(StaticTokenProvider("t"))
	require.Equal(t, "auth", ic.Name())
}

func TestClientInterceptor_AttachToken(t *testing.T) {
	ci := ClientInterceptor(StaticTokenProvider("tok123"))
	inner := ci.(*clientInterceptor)

	ctx, err := inner.attachToken(t.Context())
	require.NoError(t, err)

	md, ok := grpcmetadata.FromOutgoingContext(ctx)
	require.True(t, ok, "no outgoing metadata")
	auth := md.Get("authorization")
	require.NotEqual(t, 0, len(auth))
	require.Equal(t, "Bearer tok123", auth[0])
}

func TestClientInterceptor_EmptyToken(t *testing.T) {
	ci := ClientInterceptor(StaticTokenProvider(""))
	inner := ci.(*clientInterceptor)

	ctx, err := inner.attachToken(t.Context())
	require.NoError(t, err)

	// No metadata should be attached for empty token
	_, ok := grpcmetadata.FromOutgoingContext(ctx)
	require.False(t, ok, "should not have outgoing metadata for empty token")
}

func TestClientInterceptor_CustomHeader(t *testing.T) {
	ci := ClientInterceptor(StaticTokenProvider("key"), WithHeaderName("x-api-key"), WithScheme(""))
	inner := ci.(*clientInterceptor)

	ctx, err := inner.attachToken(t.Context())
	require.NoError(t, err)

	md, ok := grpcmetadata.FromOutgoingContext(ctx)
	require.True(t, ok, "no outgoing metadata")
	val := md.Get("x-api-key")
	require.NotEqual(t, 0, len(val))
	require.Equal(t, "key", val[0])
}

func TestClientInterceptor_ExistingMetadata(t *testing.T) {
	ci := ClientInterceptor(StaticTokenProvider("tok"))
	inner := ci.(*clientInterceptor)

	ctx := grpcmetadata.NewOutgoingContext(t.Context(), grpcmetadata.Pairs("x-custom", "val"))
	ctx, err := inner.attachToken(ctx)
	require.NoError(t, err)

	md, _ := grpcmetadata.FromOutgoingContext(ctx)
	v := md.Get("x-custom")
	require.NotEqual(t, 0, len(v))
	require.Equal(t, "val", v[0])
	v = md.Get("authorization")
	require.NotEqual(t, 0, len(v))
	require.Equal(t, "Bearer tok", v[0])
}

func BenchmarkClientInterceptor_AttachToken(b *testing.B) {
	ci := ClientInterceptor(StaticTokenProvider("tok"))
	inner := ci.(*clientInterceptor)
	for b.Loop() {
		inner.attachToken(b.Context()) //nolint:errcheck
	}
}
