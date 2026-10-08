// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldmask"

	pbfieldmask "github.com/altessa-s/go-atlas/domain/proto/fieldmask"
	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

// maskSeenByHandler runs one unary call and returns what ReadMaskFromContext
// reports inside the handler.
func maskSeenByHandler(t *testing.T, ctx context.Context, uni grpc.UnaryServerInterceptor, method string, req any) (*fieldmaskpb.FieldMask, bool) {
	t.Helper()

	var (
		mask *fieldmaskpb.FieldMask
		ok   bool
	)
	handler := func(hctx context.Context, _ any) (any, error) {
		mask, ok = fieldmask.ReadMaskFromContext(hctx)
		return &pb.Resource{}, nil
	}
	_, err := uni(ctx, req, &grpc.UnaryServerInfo{FullMethod: method}, handler)
	require.NoError(t, err)
	return mask, ok
}

func TestReadMaskFromContext(t *testing.T) {
	t.Parallel()

	withHeader := func(t *testing.T, value string) context.Context {
		t.Helper()
		return metadata.NewIncomingContext(t.Context(), metadata.Pairs(pbfieldmask.DefaultMetadataReadMaskHeader, value))
	}

	t.Run("request field mask", func(t *testing.T) {
		t.Parallel()
		req := &pb.GetResourceRequest{ReadMask: &fieldmaskpb.FieldMask{Paths: []string{"name", "id"}}}
		mask, ok := maskSeenByHandler(t, t.Context(), newUnary(t), "/x.v1.X/GetResource", req)
		require.True(t, ok)
		require.Equal(t, []string{"name", "id"}, mask.GetPaths())
	})

	t.Run("metadata header", func(t *testing.T) {
		t.Parallel()
		mask, ok := maskSeenByHandler(t, withHeader(t, "name"), newUnary(t), "/x.v1.X/GetResource", &pb.GetResourceRequest{})
		require.True(t, ok)
		require.Equal(t, []string{"name"}, mask.GetPaths())
	})

	t.Run("star means no mask", func(t *testing.T) {
		t.Parallel()
		_, ok := maskSeenByHandler(t, withHeader(t, "*"), newUnary(t), "/x.v1.X/GetResource", &pb.GetResourceRequest{})
		require.False(t, ok)
	})

	t.Run("absent mask", func(t *testing.T) {
		t.Parallel()
		_, ok := maskSeenByHandler(t, t.Context(), newUnary(t), "/x.v1.X/GetResource", &pb.GetResourceRequest{})
		require.False(t, ok)
	})

	t.Run("not a read method", func(t *testing.T) {
		t.Parallel()
		_, ok := maskSeenByHandler(t, withHeader(t, "name"), newUnary(t), "/x.v1.X/CreateResource", &pb.GetResourceRequest{})
		require.False(t, ok)
	})

	t.Run("ignored method", func(t *testing.T) {
		t.Parallel()
		uni := newUnary(t, fieldmask.WithIgnoreMethods("/x.v1.X/GetResource"))
		_, ok := maskSeenByHandler(t, withHeader(t, "name"), uni, "/x.v1.X/GetResource", &pb.GetResourceRequest{})
		require.False(t, ok)
	})

	t.Run("captured even when response filtering is off", func(t *testing.T) {
		t.Parallel()
		uni := newUnary(t, fieldmask.WithSkipReadMask())
		mask, ok := maskSeenByHandler(t, withHeader(t, "name"), uni, "/x.v1.X/GetResource", &pb.GetResourceRequest{})
		require.True(t, ok)
		require.Equal(t, []string{"name"}, mask.GetPaths())
	})

	t.Run("returned mask is a copy", func(t *testing.T) {
		t.Parallel()
		var second *fieldmaskpb.FieldMask
		handler := func(hctx context.Context, _ any) (any, error) {
			first, _ := fieldmask.ReadMaskFromContext(hctx)
			first.Paths[0] = "mutated"
			second, _ = fieldmask.ReadMaskFromContext(hctx)
			return &pb.Resource{}, nil
		}
		_, err := newUnary(t)(withHeader(t, "name"), &pb.GetResourceRequest{},
			&grpc.UnaryServerInfo{FullMethod: "/x.v1.X/GetResource"}, handler)
		require.NoError(t, err)
		require.Equal(t, []string{"name"}, second.GetPaths())
	})

	t.Run("no interceptor", func(t *testing.T) {
		t.Parallel()
		_, ok := fieldmask.ReadMaskFromContext(t.Context())
		require.False(t, ok)
	})
}
