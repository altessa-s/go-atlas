// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/fieldmask/extract"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldmask"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

func TestServerInterceptor_StrictModeRejectsInvalidMasks(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		mask        *fieldmaskpb.FieldMask
		wantPath    string
		wantMessage string
	}{
		{name: "missing", wantPath: "update_mask", wantMessage: "required"},
		{name: "empty", mask: &fieldmaskpb.FieldMask{}, wantPath: "update_mask", wantMessage: "at least one"},
		{name: "unknown proto path", mask: &fieldmaskpb.FieldMask{Paths: []string{"displayName"}}, wantPath: "displayName", wantMessage: "does not exist"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			uni := newUnary(t, fieldmask.WithStrictMode())
			resource := &pb.Resource{Name: "unchanged", Description: "keep", Profile: &pb.Profile{DisplayName: "before"}}
			req := &pb.UpdateResourceRequest{Resource: resource, UpdateMask: tc.mask}
			called := false
			_, err := uni(t.Context(), req, &grpc.UnaryServerInfo{FullMethod: "/x.v1.X/UpdateResource"}, func(_ context.Context, _ any) (any, error) {
				called = true
				return &pb.Resource{}, nil
			})
			require.False(t, called, "invalid request must not reach the handler")
			require.Equal(t, codes.InvalidArgument, status.Code(err))
			st, ok := status.FromError(err)
			require.True(t, ok)
			var badRequest *errdetails.BadRequest
			for _, detail := range st.Details() {
				if typed, ok := detail.(*errdetails.BadRequest); ok {
					badRequest = typed
				}
			}
			require.NotNil(t, badRequest)
			require.NotEmpty(t, badRequest.GetFieldViolations())
			require.Equal(t, "update_mask", badRequest.GetFieldViolations()[0].GetField())
			require.Contains(t, badRequest.GetFieldViolations()[0].GetDescription(), tc.wantPath)
			require.Contains(t, badRequest.GetFieldViolations()[0].GetDescription(), tc.wantMessage)
			require.Equal(t, "unchanged", resource.GetName(), "validation must precede mutation")
			require.Equal(t, "keep", resource.GetDescription())
		})
	}
}

// TestServerInterceptor_StrictModeWildcardExpandsMutableFields pins the strict
// expansion of "*": the handler gets the mask of every field but the
// OUTPUT_ONLY ones, and the OUTPUT_ONLY client values are cleared.
func TestServerInterceptor_StrictModeWildcardExpandsMutableFields(t *testing.T) {
	t.Parallel()
	uni := newUnary(t, fieldmask.WithStrictMode())
	req := &pb.UpdateResourceRequest{
		Resource:   &pb.Resource{Name: "name", Description: "description", CreateTime: "server", Profile: &pb.Profile{DisplayName: "display", UpdatedAt: "server"}},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"*"}},
	}
	seen, _, err := runUnary(t, uni, "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
	require.NoError(t, err)
	got := seen.(*pb.UpdateResourceRequest)
	// "*" expands to every field but the OUTPUT_ONLY ones; the unset IMMUTABLE
	// and IDENTIFIER fields keep their stored values, so they are left out,
	// and collections are replaced whole.
	require.ElementsMatch(t, []string{
		"name", "description", "password", "profile.display_name", "profile.secret",
	}, got.GetUpdateMask().GetPaths())
	require.Empty(t, got.GetResource().GetCreateTime())
	require.Empty(t, got.GetResource().GetTenantId())
	require.Empty(t, got.GetResource().GetProfile().GetUpdatedAt())
}

func TestServerInterceptor_StrictModeKeepsExtractorOverrides(t *testing.T) {
	t.Parallel()
	uni := newUnary(t,
		fieldmask.WithStrictMode(),
		fieldmask.WithUpdateExtractor("/x.v1.X/UpdateNestedResource", extract.NewUpdateFunc(
			func(r *pb.NestedUpdateResourceRequest) *fieldmaskpb.FieldMask { return r.GetOptions().GetUpdateMask() },
			func(r *pb.NestedUpdateResourceRequest) *pb.Resource { return r.GetResource() },
			func(r *pb.NestedUpdateResourceRequest, m *fieldmaskpb.FieldMask) { r.Options.UpdateMask = m },
		)),
	)
	req := &pb.NestedUpdateResourceRequest{
		Resource: &pb.Resource{Name: "name", Description: "description"},
		Options:  &pb.NestedUpdateResourceRequest_Options{UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"description"}}},
	}
	seen, _, err := runUnary(t, uni, "/x.v1.X/UpdateNestedResource", req, &pb.Resource{}, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"description"}, seen.(*pb.NestedUpdateResourceRequest).GetOptions().GetUpdateMask().GetPaths())
}

func TestServerInterceptor_StrictModeRequiresCanonicalMask(t *testing.T) {
	t.Parallel()
	uni := newUnary(t, fieldmask.WithStrictMode())
	req := &pb.NestedUpdateResourceRequest{Resource: &pb.Resource{Name: "name"}}
	_, _, err := runUnary(t, uni, "/x.v1.X/UpdateNestedResource", req, &pb.Resource{}, nil)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

// TestServerInterceptor_StrictModeDropsOutputOnlyPaths pins AIP-203 in strict
// mode: an OUTPUT_ONLY path, top-level or nested, is dropped from the mask
// rather than rejected, and its client value is cleared.
func TestServerInterceptor_StrictModeDropsOutputOnlyPaths(t *testing.T) {
	t.Parallel()
	uni := newUnary(t, fieldmask.WithStrictMode())
	req := &pb.UpdateResourceRequest{
		Resource:   &pb.Resource{Description: "d", CreateTime: "client", Profile: &pb.Profile{DisplayName: "x", UpdatedAt: "client"}},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"description", "create_time", "profile.updated_at"}},
	}
	seen, _, err := runUnary(t, uni, "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
	require.NoError(t, err)
	got := seen.(*pb.UpdateResourceRequest)
	require.Equal(t, []string{"description"}, got.GetUpdateMask().GetPaths())
	require.Empty(t, got.GetResource().GetCreateTime())
	require.Empty(t, got.GetResource().GetProfile().GetUpdatedAt())
}

// TestServerInterceptor_StrictModeDroppedMaskIsNotSynthesized pins that a
// non-empty update_mask whose every path strict validation drops stays a
// no-op, so dropping an OUTPUT_ONLY path never makes the other populated
// fields writable.
func TestServerInterceptor_StrictModeDroppedMaskIsNotSynthesized(t *testing.T) {
	t.Parallel()
	uni := newUnary(t, fieldmask.WithStrictMode())
	req := &pb.UpdateResourceRequest{
		Resource:   &pb.Resource{Name: "n", Description: "d", CreateTime: "client"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"create_time"}},
	}
	seen, _, err := runUnary(t, uni, "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
	require.NoError(t, err)
	require.Empty(t, seen.(*pb.UpdateResourceRequest).GetUpdateMask().GetPaths(), "nothing is writable")

	// Without strict mode an explicit empty mask is still a no-op.
	uni = newUnary(t)
	req = &pb.UpdateResourceRequest{Resource: &pb.Resource{Name: "n", Description: "d"}, UpdateMask: &fieldmaskpb.FieldMask{}}
	seen, _, err = runUnary(t, uni, "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
	require.NoError(t, err)
	require.Empty(t, seen.(*pb.UpdateResourceRequest).GetUpdateMask().GetPaths())
}

// TestServerInterceptor_StrictModeRejectsNilMaskFromExtractor pins that a
// custom extractor reporting a nil mask with ok=true is refused in strict
// mode as a missing update_mask, rather than synthesized from the populated
// fields.
func TestServerInterceptor_StrictModeRejectsNilMaskFromExtractor(t *testing.T) {
	t.Parallel()
	extractor := func(_ context.Context, req proto.Message) (*fieldmaskpb.FieldMask, proto.Message, func(*fieldmaskpb.FieldMask), bool) {
		r := req.(*pb.UpdateResourceRequest)
		return nil, r.GetResource(), func(m *fieldmaskpb.FieldMask) { r.UpdateMask = m }, true
	}
	for _, opts := range [][]fieldmask.Option{
		{fieldmask.WithStrictMode(), fieldmask.WithUpdateExtractor("/x.v1.X/UpdateResource", extractor)},
		{fieldmask.WithStrictMode(), fieldmask.WithDefaultUpdateExtractor(extractor)},
	} {
		req := &pb.UpdateResourceRequest{Resource: &pb.Resource{Name: "n", Description: "d"}}
		seen, _, err := runUnary(t, newUnary(t, opts...), "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
		require.Nil(t, seen)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
		require.Contains(t, status.Convert(err).Message(), "update_mask is required")
	}
}
