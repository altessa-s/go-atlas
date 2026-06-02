// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldmask"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	pbfieldmask "github.com/altessa-s/go-atlas/domain/proto/fieldmask"
	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

// TestServerInterceptor_ResolutionPrecedence locks the three-tier resolution
// order: per-method override → global default → built-in reflection.
func TestServerInterceptor_ResolutionPrecedence(t *testing.T) {
	t.Parallel()

	t.Run("per-method override wins over global default", func(t *testing.T) {
		t.Parallel()

		var perMethodCalled, globalCalled bool
		uni := newUnary(t,
			fieldmask.WithUpdateExtractor("/x.v1.X/UpdateResource", func(context.Context, proto.Message) (
				*fieldmaskpb.FieldMask, proto.Message, func(*fieldmaskpb.FieldMask), bool,
			) {
				perMethodCalled = true
				return nil, nil, nil, false
			}),
			fieldmask.WithDefaultUpdateExtractor(func(context.Context, proto.Message) (
				*fieldmaskpb.FieldMask, proto.Message, func(*fieldmaskpb.FieldMask), bool,
			) {
				globalCalled = true
				return nil, nil, nil, false
			}),
		)

		req := &pb.UpdateResourceRequest{
			Resource:   &pb.Resource{Name: "n", TenantId: "x"},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"tenant_id"}},
		}
		_, _, err := runUnary(t, uni, "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
		require.NoError(t, err)
		require.True(t, perMethodCalled, "per-method extractor must run")
		require.False(t, globalCalled, "global default must not run when per-method matches")
	})

	t.Run("global default wins over built-in", func(t *testing.T) {
		t.Parallel()

		var globalCalled bool
		uni := newUnary(t,
			fieldmask.WithDefaultUpdateExtractor(func(context.Context, proto.Message) (
				*fieldmaskpb.FieldMask, proto.Message, func(*fieldmaskpb.FieldMask), bool,
			) {
				globalCalled = true
				return nil, nil, nil, false
			}),
		)

		// AIP-134 canonical shape: the built-in would reject tenant_id as
		// IMMUTABLE, but the global default returns ok=false → passthrough.
		req := &pb.UpdateResourceRequest{
			Resource:   &pb.Resource{Name: "n", TenantId: "x"},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"tenant_id"}},
		}
		_, _, err := runUnary(t, uni, "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
		require.NoError(t, err, "global default shadowed the built-in reflection extractor")
		require.True(t, globalCalled)
	})

	t.Run("built-in runs when no overrides", func(t *testing.T) {
		t.Parallel()

		uni := newUnary(t)
		req := &pb.UpdateResourceRequest{
			Resource:   &pb.Resource{Name: "n", TenantId: "x"},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"tenant_id"}},
		}
		_, _, err := runUnary(t, uni, "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
		require.Error(t, err)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})
}

// TestServerInterceptor_NestedUpdateMask exercises the headline use-case:
// a request whose update_mask lives inside an Options sub-message.
func TestServerInterceptor_NestedUpdateMask(t *testing.T) {
	t.Parallel()

	nestedExtractor := pbfieldmask.NewUpdateExtractor(
		func(r *pb.NestedUpdateResourceRequest) *fieldmaskpb.FieldMask { return r.GetOptions().GetUpdateMask() },
		func(r *pb.NestedUpdateResourceRequest) *pb.Resource { return r.GetResource() },
		func(r *pb.NestedUpdateResourceRequest, m *fieldmaskpb.FieldMask) {
			if r.Options == nil {
				r.Options = &pb.NestedUpdateResourceRequest_Options{}
			}
			r.Options.UpdateMask = m
		},
	)

	t.Run("immutable violation surfaces as BadRequest", func(t *testing.T) {
		t.Parallel()

		uni := newUnary(t,
			fieldmask.WithMethodKind("/x.v1.X/UpdateNestedResource", fieldmask.KindUpdate),
			fieldmask.WithUpdateExtractor("/x.v1.X/UpdateNestedResource", nestedExtractor),
		)
		req := &pb.NestedUpdateResourceRequest{
			Resource: &pb.Resource{Name: "n", TenantId: "altered"},
			Options: &pb.NestedUpdateResourceRequest_Options{
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"tenant_id"}},
			},
		}
		_, _, err := runUnary(t, uni, "/x.v1.X/UpdateNestedResource", req, &pb.Resource{}, nil)
		require.Error(t, err)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("writeback rewrites the nested mask", func(t *testing.T) {
		t.Parallel()

		uni := newUnary(t,
			fieldmask.WithMethodKind("/x.v1.X/UpdateNestedResource", fieldmask.KindUpdate),
			fieldmask.WithUpdateExtractor("/x.v1.X/UpdateNestedResource", nestedExtractor),
		)
		req := &pb.NestedUpdateResourceRequest{
			Resource: &pb.Resource{Name: "n"},
			Options: &pb.NestedUpdateResourceRequest_Options{
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name", "create_time"}},
			},
		}
		seenReq, _, err := runUnary(t, uni, "/x.v1.X/UpdateNestedResource", req, &pb.Resource{}, nil)
		require.NoError(t, err)

		got := seenReq.(*pb.NestedUpdateResourceRequest)
		require.Equal(t, []string{"name"}, got.GetOptions().GetUpdateMask().GetPaths(),
			"OUTPUT_ONLY entry must be removed from the nested writeback mask")
	})

	t.Run("passthrough when extractor not registered", func(t *testing.T) {
		t.Parallel()

		uni := newUnary(t, fieldmask.WithMethodKind("/x.v1.X/UpdateNestedResource", fieldmask.KindUpdate))
		req := &pb.NestedUpdateResourceRequest{
			Resource: &pb.Resource{Name: "n", TenantId: "altered"},
			Options: &pb.NestedUpdateResourceRequest_Options{
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"tenant_id"}},
			},
		}
		_, _, err := runUnary(t, uni, "/x.v1.X/UpdateNestedResource", req, &pb.Resource{}, nil)
		require.NoError(t, err, "built-in extractor cannot find nested mask, request passes through")
	})
}

// TestServerInterceptor_NestedReadMask covers KindRead with a nested
// read_mask.
func TestServerInterceptor_NestedReadMask(t *testing.T) {
	t.Parallel()

	nestedRead := pbfieldmask.NewReadExtractor(
		func(r *pb.NestedGetResourceRequest) *fieldmaskpb.FieldMask { return r.GetOptions().GetReadMask() },
	)

	uni := newUnary(t,
		fieldmask.WithMethodKind("/x.v1.X/GetNestedResource", fieldmask.KindRead),
		fieldmask.WithReadExtractor("/x.v1.X/GetNestedResource", nestedRead),
	)

	req := &pb.NestedGetResourceRequest{
		Name: "id-1",
		Options: &pb.NestedGetResourceRequest_Options{
			ReadMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
		},
	}
	resp := &pb.Resource{Id: "id-1", Name: "kept", Description: "dropped"}

	_, gotResp, err := runUnary(t, uni, "/x.v1.X/GetNestedResource", req, resp, nil)
	require.NoError(t, err)

	got := gotResp.(*pb.Resource)
	require.Equal(t, "kept", got.GetName())
	require.Empty(t, got.GetDescription())
	require.Empty(t, got.GetId())
}

// TestServerInterceptor_DefaultExtractorOverride checks the global default
// applies when no per-method override matches.
func TestServerInterceptor_DefaultExtractorOverride(t *testing.T) {
	t.Parallel()

	t.Run("default update extractor handles nested shape", func(t *testing.T) {
		t.Parallel()

		uni := newUnary(t,
			fieldmask.WithMethodKind("/x.v1.X/UpdateNestedResource", fieldmask.KindUpdate),
			fieldmask.WithDefaultUpdateExtractor(pbfieldmask.NewUpdateExtractor(
				func(r *pb.NestedUpdateResourceRequest) *fieldmaskpb.FieldMask { return r.GetOptions().GetUpdateMask() },
				func(r *pb.NestedUpdateResourceRequest) *pb.Resource { return r.GetResource() },
				func(r *pb.NestedUpdateResourceRequest, m *fieldmaskpb.FieldMask) { r.Options.UpdateMask = m },
			)),
		)

		req := &pb.NestedUpdateResourceRequest{
			Resource: &pb.Resource{Name: "n", TenantId: "altered"},
			Options: &pb.NestedUpdateResourceRequest_Options{
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"tenant_id"}},
			},
		}
		_, _, err := runUnary(t, uni, "/x.v1.X/UpdateNestedResource", req, &pb.Resource{}, nil)
		require.Error(t, err)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("default read extractor handles nested shape", func(t *testing.T) {
		t.Parallel()

		uni := newUnary(t,
			fieldmask.WithMethodKind("/x.v1.X/GetNestedResource", fieldmask.KindRead),
			fieldmask.WithDefaultReadExtractor(pbfieldmask.NewReadExtractor(
				func(r *pb.NestedGetResourceRequest) *fieldmaskpb.FieldMask { return r.GetOptions().GetReadMask() },
			)),
		)

		req := &pb.NestedGetResourceRequest{
			Name: "id-1",
			Options: &pb.NestedGetResourceRequest_Options{
				ReadMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
			},
		}
		resp := &pb.Resource{Id: "id-1", Name: "kept", Description: "dropped"}

		_, gotResp, err := runUnary(t, uni, "/x.v1.X/GetNestedResource", req, resp, nil)
		require.NoError(t, err)
		require.Empty(t, gotResp.(*pb.Resource).GetDescription())
	})
}

// TestServerInterceptor_MetadataReadMask covers the AIP-157 modern path:
// the built-in read extractor reads x-goog-fieldmask metadata before
// falling back to the deprecated request-message read_mask.
func TestServerInterceptor_MetadataReadMask(t *testing.T) {
	t.Parallel()

	runWithMetadata := func(t *testing.T, uni grpc.UnaryServerInterceptor, md metadata.MD, req, resp any) (any, error) {
		t.Helper()

		ctx := t.Context()
		if md != nil {
			ctx = metadata.NewIncomingContext(ctx, md)
		}

		info := &grpc.UnaryServerInfo{FullMethod: "/x.v1.X/GetResource"}
		handler := func(_ context.Context, _ any) (any, error) {
			return resp, nil
		}
		return uni(ctx, req, info, handler)
	}

	t.Run("metadata wins over absent request-field mask", func(t *testing.T) {
		t.Parallel()

		uni := newUnary(t)
		md := metadata.Pairs(pbfieldmask.DefaultMetadataReadMaskHeader, "name")
		req := &pb.GetResourceRequest{Name: "id-1"}
		resp := &pb.Resource{Id: "id-1", Name: "kept", Description: "dropped"}

		got, err := runWithMetadata(t, uni, md, req, resp)
		require.NoError(t, err)

		out := got.(*pb.Resource)
		require.Equal(t, "kept", out.GetName())
		require.Empty(t, out.GetDescription())
		require.Empty(t, out.GetId())
	})

	t.Run("metadata wins over request-field mask", func(t *testing.T) {
		t.Parallel()

		uni := newUnary(t)
		md := metadata.Pairs(pbfieldmask.DefaultMetadataReadMaskHeader, "name")
		req := &pb.GetResourceRequest{
			Name:     "id-1",
			ReadMask: &fieldmaskpb.FieldMask{Paths: []string{"description"}},
		}
		resp := &pb.Resource{Name: "kept", Description: "dropped"}

		got, err := runWithMetadata(t, uni, md, req, resp)
		require.NoError(t, err)

		out := got.(*pb.Resource)
		require.Equal(t, "kept", out.GetName())
		require.Empty(t, out.GetDescription(), "metadata header overrides request-field mask")
	})

	t.Run("metadata absent falls back to request-field mask", func(t *testing.T) {
		t.Parallel()

		uni := newUnary(t)
		req := &pb.GetResourceRequest{
			Name:     "id-1",
			ReadMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
		}
		resp := &pb.Resource{Name: "kept", Description: "dropped"}

		got, err := runWithMetadata(t, uni, nil, req, resp)
		require.NoError(t, err)

		out := got.(*pb.Resource)
		require.Equal(t, "kept", out.GetName())
		require.Empty(t, out.GetDescription())
	})

	t.Run("metadata star means all fields (passthrough)", func(t *testing.T) {
		t.Parallel()

		uni := newUnary(t)
		md := metadata.Pairs(pbfieldmask.DefaultMetadataReadMaskHeader, "*")
		req := &pb.GetResourceRequest{Name: "id-1"}
		resp := &pb.Resource{Name: "kept", Description: "also kept"}

		got, err := runWithMetadata(t, uni, md, req, resp)
		require.NoError(t, err)

		out := got.(*pb.Resource)
		require.Equal(t, "kept", out.GetName())
		require.Equal(t, "also kept", out.GetDescription())
	})

	t.Run("custom header via WithMetadataReadMaskHeader", func(t *testing.T) {
		t.Parallel()

		uni := newUnary(t, fieldmask.WithMetadataReadMaskHeader("x-custom-fieldmask"))
		md := metadata.Pairs("x-custom-fieldmask", "name")
		req := &pb.GetResourceRequest{Name: "id-1"}
		resp := &pb.Resource{Name: "kept", Description: "dropped"}

		got, err := runWithMetadata(t, uni, md, req, resp)
		require.NoError(t, err)

		out := got.(*pb.Resource)
		require.Equal(t, "kept", out.GetName())
		require.Empty(t, out.GetDescription())
	})

	t.Run("default header ignored when custom header is set", func(t *testing.T) {
		t.Parallel()

		uni := newUnary(t, fieldmask.WithMetadataReadMaskHeader("x-custom-fieldmask"))
		md := metadata.Pairs(pbfieldmask.DefaultMetadataReadMaskHeader, "name")
		req := &pb.GetResourceRequest{Name: "id-1"}
		resp := &pb.Resource{Name: "kept", Description: "also kept"}

		got, err := runWithMetadata(t, uni, md, req, resp)
		require.NoError(t, err)

		out := got.(*pb.Resource)
		require.Equal(t, "kept", out.GetName())
		require.Equal(t, "also kept", out.GetDescription(),
			"default header must not be consulted once a custom header is configured")
	})
}

// TestServerInterceptor_ExtractorOptions_NilGuards confirms the empty-method
// / nil-fn guards do not panic.
func TestServerInterceptor_ExtractorOptions_NilGuards(t *testing.T) {
	t.Parallel()

	_ = fieldmask.ServerInterceptor(
		fieldmask.WithUpdateExtractor("", func(context.Context, proto.Message) (
			*fieldmaskpb.FieldMask, proto.Message, func(*fieldmaskpb.FieldMask), bool,
		) {
			return nil, nil, nil, false
		}),
		fieldmask.WithUpdateExtractor("/x.v1.X/UpdateResource", nil),
		fieldmask.WithReadExtractor("/x.v1.X/GetResource", nil),
		fieldmask.WithDefaultUpdateExtractor(nil),
		fieldmask.WithDefaultReadExtractor(nil),
	)
}
