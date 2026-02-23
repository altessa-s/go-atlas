// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"context"
	"testing"

	grpcmetadata "google.golang.org/grpc/metadata"
)

func TestStaticTokenProvider(t *testing.T) {
	p := StaticTokenProvider("my-token")
	tok, err := p.Token(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if tok != "my-token" {
		t.Fatalf("token = %q", tok)
	}
}

func TestTokenProviderFunc(t *testing.T) {
	f := TokenProviderFunc(func(ctx context.Context) (string, error) {
		return "dynamic", nil
	})
	tok, err := f.Token(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if tok != "dynamic" {
		t.Fatalf("token = %q", tok)
	}
}

func TestClientInterceptor_Name(t *testing.T) {
	ic := ClientInterceptor(StaticTokenProvider("t"))
	if ic.Name() != "auth" {
		t.Fatalf("Name() = %q", ic.Name())
	}
}

func TestClientInterceptor_AttachToken(t *testing.T) {
	ci := ClientInterceptor(StaticTokenProvider("tok123"))
	inner := ci.(*clientInterceptor)

	ctx, err := inner.attachToken(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	md, ok := grpcmetadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("no outgoing metadata")
	}
	auth := md.Get("authorization")
	if len(auth) == 0 || auth[0] != "Bearer tok123" {
		t.Fatalf("authorization = %v", auth)
	}
}

func TestClientInterceptor_EmptyToken(t *testing.T) {
	ci := ClientInterceptor(StaticTokenProvider(""))
	inner := ci.(*clientInterceptor)

	ctx, err := inner.attachToken(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	// No metadata should be attached for empty token
	_, ok := grpcmetadata.FromOutgoingContext(ctx)
	if ok {
		t.Fatal("should not have outgoing metadata for empty token")
	}
}

func TestClientInterceptor_CustomHeader(t *testing.T) {
	ci := ClientInterceptor(StaticTokenProvider("key"), WithHeaderName("x-api-key"), WithScheme(""))
	inner := ci.(*clientInterceptor)

	ctx, err := inner.attachToken(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	md, ok := grpcmetadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("no outgoing metadata")
	}
	val := md.Get("x-api-key")
	if len(val) == 0 || val[0] != "key" {
		t.Fatalf("x-api-key = %v", val)
	}
}

func TestClientInterceptor_ExistingMetadata(t *testing.T) {
	ci := ClientInterceptor(StaticTokenProvider("tok"))
	inner := ci.(*clientInterceptor)

	ctx := grpcmetadata.NewOutgoingContext(t.Context(), grpcmetadata.Pairs("x-custom", "val"))
	ctx, err := inner.attachToken(ctx)
	if err != nil {
		t.Fatal(err)
	}

	md, _ := grpcmetadata.FromOutgoingContext(ctx)
	if v := md.Get("x-custom"); len(v) == 0 || v[0] != "val" {
		t.Fatal("existing metadata should be preserved")
	}
	if v := md.Get("authorization"); len(v) == 0 || v[0] != "Bearer tok" {
		t.Fatal("token should be added")
	}
}

func BenchmarkClientInterceptor_AttachToken(b *testing.B) {
	ci := ClientInterceptor(StaticTokenProvider("tok"))
	inner := ci.(*clientInterceptor)
	for b.Loop() {
		inner.attachToken(b.Context()) //nolint:errcheck
	}
}
