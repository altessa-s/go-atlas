// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package requestid

import (
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc/metadata"

	internalrequestid "github.com/altessa-s/go-atlas/transport/internal/requestid"
)

func TestGrpcHeaderGetter(t *testing.T) {
	md := metadata.Pairs("x-request-id", "abc-123")
	g := &grpcHeaderGetter{md: md}

	got := g.GetHeader("x-request-id")
	require.Equal(t, "abc-123", got)
	got = g.GetHeader("nonexistent")
	require.Equal(t, "", got)
}

func TestGrpcHeaderGetter_NilMD(t *testing.T) {
	g := &grpcHeaderGetter{md: nil}
	got := g.GetHeader("any")
	require.Equal(t, "", got)
}

func TestContext_RoundTrip(t *testing.T) {
	ctx := t.Context()
	got := FromContext(ctx)
	require.Equal(t, "", got)

	ctx = NewContext(ctx, "test-id")
	got = FromContext(ctx)
	require.Equal(t, "test-id", got)
}

func TestServerInterceptor_Dependencies(t *testing.T) {
	i := &interceptor{}
	deps := i.Dependencies()
	require.Len(t, deps, 1)
	require.Equal(t, "metadata", deps[0])
}

func TestClientAttachRequestID_NoOutgoingMD(t *testing.T) {
	gen := internalrequestid.NewGenerator()
	i := &interceptor{gen: gen}

	const reqID = "018f2a3c-1111-4222-8333-444455556666"
	ctx := i.clientAttachRequestID(NewContext(t.Context(), reqID))

	md, ok := metadata.FromOutgoingContext(ctx)
	require.True(t, ok)
	require.Equal(t, []string{reqID}, md.Get(gen.HeaderName()))
	require.Len(t, md, 1)
}

func TestClientAttachRequestID_MergesExistingMD(t *testing.T) {
	gen := internalrequestid.NewGenerator()
	i := &interceptor{gen: gen}

	const reqID = "018f2a3c-1111-4222-8333-444455556666"
	existing := metadata.Pairs("authorization", "bearer token")
	ctx := metadata.NewOutgoingContext(NewContext(t.Context(), reqID), existing)

	outCtx := i.clientAttachRequestID(ctx)

	md, ok := metadata.FromOutgoingContext(outCtx)
	require.True(t, ok)
	require.Equal(t, []string{reqID}, md.Get(gen.HeaderName()))
	require.Equal(t, []string{"bearer token"}, md.Get("authorization"))
	// The caller's metadata map must not be mutated.
	require.Empty(t, existing.Get(gen.HeaderName()))
}

// TestClientAttachRequestID_MetadataSurvivesLaterCalls pins the fix for the
// pooled-metadata recycle bug: the metadata attached to one outgoing context
// must not be cleared or overwritten by a later attach on another context.
func TestClientAttachRequestID_MetadataSurvivesLaterCalls(t *testing.T) {
	gen := internalrequestid.NewGenerator()
	i := &interceptor{gen: gen}

	const firstID = "018f2a3c-1111-4222-8333-444455556666"
	const secondID = "018f2a3c-7777-4888-8999-000011112222"

	firstCtx := i.clientAttachRequestID(NewContext(t.Context(), firstID))
	_ = i.clientAttachRequestID(NewContext(t.Context(), secondID))

	md, ok := metadata.FromOutgoingContext(firstCtx)
	require.True(t, ok)
	require.Equal(t, []string{firstID}, md.Get(gen.HeaderName()))
}
