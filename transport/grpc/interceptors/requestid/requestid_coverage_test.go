// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package requestid

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	internalrequestid "github.com/altessa-s/go-atlas/internal/transport/requestid"
)

const validUUID = "018f2a3c-1111-4222-8333-444455556666"

type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f *fakeServerStream) Context() context.Context { return f.ctx }

func (f *fakeServerStream) SetHeader(metadata.MD) error { return nil }

func TestName_ID(t *testing.T) {
	t.Parallel()

	require.Equal(t, "requestid", Name())
}

func TestServerUnaryInterceptor_ValidIncomingID(t *testing.T) {
	t.Parallel()

	gen := internalrequestid.NewGenerator()
	inter := ServerUnaryInterceptor(gen)
	info := &grpc.UnaryServerInfo{FullMethod: "/svc/Method"}

	md := metadata.New(map[string]string{gen.HeaderName(): validUUID})
	ctx := metadata.NewIncomingContext(t.Context(), md)

	var gotID string
	resp, err := inter(ctx, "req", info, func(handlerCtx context.Context, _ any) (any, error) {
		gotID = FromContext(handlerCtx)
		return "ok", nil
	})

	require.NoError(t, err)
	require.Equal(t, "ok", resp)
	require.Equal(t, validUUID, gotID)
}

func TestServerUnaryInterceptor_GeneratesWhenMissing(t *testing.T) {
	t.Parallel()

	gen := internalrequestid.NewGenerator(internalrequestid.WithGenerateIfMissing(true))
	inter := ServerUnaryInterceptor(gen)
	info := &grpc.UnaryServerInfo{FullMethod: "/svc/Method"}

	var gotID string
	_, err := inter(t.Context(), "req", info, func(handlerCtx context.Context, _ any) (any, error) {
		gotID = FromContext(handlerCtx)
		return "ok", nil
	})

	require.NoError(t, err)
	require.NotEmpty(t, gotID)
}

func TestServerStreamInterceptor_ValidIncomingID(t *testing.T) {
	t.Parallel()

	gen := internalrequestid.NewGenerator()
	inter := ServerStreamInterceptor(gen)
	info := &grpc.StreamServerInfo{FullMethod: "/svc/Stream"}

	md := metadata.New(map[string]string{gen.HeaderName(): validUUID})
	ctx := metadata.NewIncomingContext(t.Context(), md)
	stream := &fakeServerStream{ctx: ctx}

	var gotID string
	err := inter(nil, stream, info, func(_ any, s grpc.ServerStream) error {
		gotID = FromContext(s.Context())
		return nil
	})

	require.NoError(t, err)
	require.Equal(t, validUUID, gotID)
}

func TestClientUnaryInterceptor_AttachesRequestID(t *testing.T) {
	t.Parallel()

	gen := internalrequestid.NewGenerator()
	inter := ClientUnaryInterceptor(gen)

	var invokerCtx context.Context
	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		invokerCtx = ctx
		return nil
	}

	err := inter(NewContext(t.Context(), validUUID), "/svc/Method", "req", "reply", nil, invoker)

	require.NoError(t, err)
	md, ok := metadata.FromOutgoingContext(invokerCtx)
	require.True(t, ok)
	require.Equal(t, []string{validUUID}, md.Get(gen.HeaderName()))
}

func TestClientStreamInterceptor_AttachesRequestID(t *testing.T) {
	t.Parallel()

	gen := internalrequestid.NewGenerator()
	inter := ClientStreamInterceptor(gen)

	var streamerCtx context.Context
	streamer := func(ctx context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		streamerCtx = ctx
		return nil, nil
	}

	_, err := inter(NewContext(t.Context(), validUUID), &grpc.StreamDesc{}, nil, "/svc/Stream", streamer)

	require.NoError(t, err)
	md, ok := metadata.FromOutgoingContext(streamerCtx)
	require.True(t, ok)
	require.Equal(t, []string{validUUID}, md.Get(gen.HeaderName()))
}

func TestClientInterceptor_Name(t *testing.T) {
	t.Parallel()

	gen := internalrequestid.NewGenerator()
	require.Equal(t, "requestid", ClientInterceptor(gen).Name())
	require.Equal(t, "requestid", ServerInterceptor(gen).Name())
}

func TestServerUnaryInterceptor_GenerationDisabled(t *testing.T) {
	t.Parallel()

	gen := internalrequestid.NewGenerator(internalrequestid.WithGenerateIfMissing(false))
	inter := ServerUnaryInterceptor(gen)
	info := &grpc.UnaryServerInfo{FullMethod: "/svc/Method"}
	noop := func(context.Context, any) (any, error) { return "ok", nil }

	tests := []struct {
		name string
		ctx  context.Context
	}{
		{"missing_id", t.Context()},
		{"invalid_id", metadata.NewIncomingContext(t.Context(), metadata.Pairs(gen.HeaderName(), "not-a-uuid"))},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := inter(tc.ctx, "req", info, noop)
			require.Equal(t, codes.InvalidArgument, status.Code(err))
		})
	}

	t.Run("valid_id_passes", func(t *testing.T) {
		t.Parallel()
		ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs(gen.HeaderName(), validUUID))
		var gotID string
		_, err := inter(ctx, "req", info, func(handlerCtx context.Context, _ any) (any, error) {
			gotID = FromContext(handlerCtx)
			return "ok", nil
		})
		require.NoError(t, err)
		require.Equal(t, validUUID, gotID)
	})
}

func TestClientUnaryInterceptor_GenerationDisabled(t *testing.T) {
	t.Parallel()

	gen := internalrequestid.NewGenerator(internalrequestid.WithGenerateIfMissing(false))
	inter := ClientUnaryInterceptor(gen)

	var invokerCtx context.Context
	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		invokerCtx = ctx
		return nil
	}

	require.NoError(t, inter(t.Context(), "/svc/Method", "req", "reply", nil, invoker))
	_, ok := metadata.FromOutgoingContext(invokerCtx)
	require.False(t, ok, "no request ID metadata is attached when generation is disabled")
	require.Empty(t, FromContext(invokerCtx))
}
