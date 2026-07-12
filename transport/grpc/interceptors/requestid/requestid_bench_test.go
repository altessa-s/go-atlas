// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package requestid

import (
	"testing"

	"google.golang.org/grpc/metadata"

	internalrequestid "github.com/altessa-s/go-atlas/transport/internal/requestid"
)

func BenchmarkClientAttachRequestID(b *testing.B) {
	gen := internalrequestid.NewGenerator()
	i := &interceptor{gen: gen}
	ctx := NewContext(b.Context(), "018f2a3c-1111-4222-8333-444455556666")

	b.ReportAllocs()
	for b.Loop() {
		i.clientAttachRequestID(ctx)
	}
}

func BenchmarkClientAttachRequestID_ExistingMD(b *testing.B) {
	gen := internalrequestid.NewGenerator()
	i := &interceptor{gen: gen}
	ctx := metadata.NewOutgoingContext(
		NewContext(b.Context(), "018f2a3c-1111-4222-8333-444455556666"),
		metadata.Pairs("authorization", "bearer token"),
	)

	b.ReportAllocs()
	for b.Loop() {
		i.clientAttachRequestID(ctx)
	}
}

func BenchmarkContextWithRequestID(b *testing.B) {
	gen := internalrequestid.NewGenerator()
	i := &interceptor{gen: gen}
	ctx := metadata.NewIncomingContext(
		b.Context(),
		metadata.Pairs(gen.HeaderName(), "018f2a3c-1111-4222-8333-444455556666"),
	)

	b.ReportAllocs()
	for b.Loop() {
		_, _ = i.contextWithRequestID(ctx)
	}
}

func BenchmarkGrpcHeaderGetter_GetHeader(b *testing.B) {
	md := metadata.Pairs("x-request-id", "abc-123")
	g := &grpcHeaderGetter{md: md}

	b.ReportAllocs()
	for b.Loop() {
		g.GetHeader("x-request-id")
	}
}
