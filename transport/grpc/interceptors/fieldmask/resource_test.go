// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/proto-gen-go/io/altessa/badrequest/v1"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldmask"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

func listResponse() *pb.ListResourcesResponse {
	return &pb.ListResourcesResponse{
		Resources: []*pb.Resource{
			{Id: "1", Name: "first", Description: "one", Aliases: []*pb.Profile{{DisplayName: "a"}}},
			{Id: "2", Name: "second", Description: "two"},
		},
		NextPageToken: "next",
		TotalSize:     2,
		Unreachable:   []string{"zone-b"},
	}
}

func listRequest(paths ...string) *pb.ListResourcesRequest {
	req := &pb.ListResourcesRequest{PageSize: 2}
	if paths != nil {
		req.ReadMask = &fieldmaskpb.FieldMask{Paths: paths}
	}
	return req
}

const (
	listMethod   = pb.ResourceService_ListResources_FullMethodName
	getMethod    = pb.ResourceService_GetResource_FullMethodName
	searchMethod = pb.ResourceService_SearchResources_FullMethodName
	pagesMethod  = pb.ResourceService_ListResourcePages_FullMethodName
)

// requireMaskViolation checks the refusal of an invalid resource-relative read mask.
func requireMaskViolation(t *testing.T, err error, field string) {
	t.Helper()
	st := status.Convert(err)
	require.Equal(t, codes.InvalidArgument, st.Code(), "got %v", err)
	require.Equal(t, "Invalid read field mask", st.Message())
	var violations []*badrequestv1.FieldViolation
	for _, d := range st.Details() {
		if v, ok := d.(*badrequestv1.BadRequest); ok {
			violations = append(violations, v.GetFieldViolations()...)
		}
	}
	require.Len(t, violations, 1)
	require.Equal(t, field, violations[0].GetFieldPath())
	require.Equal(t, "invalid field mask path", violations[0].GetMessage())
	info := errorInfo(st)
	require.NotNil(t, info)
	require.Equal(t, "INVALID_FIELD_MASK", info.GetReason())
}

func TestResourceReadMask_ListFiltersEachResource(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		paths []string
		want  func(*pb.ListResourcesResponse)
	}{
		{"fields", []string{"name", "aliases.*.display_name"}, func(r *pb.ListResourcesResponse) {
			for _, res := range r.Resources {
				res.Id, res.Description = "", ""
			}
		}},
		{"json spelling", []string{"id"}, func(r *pb.ListResourcesResponse) {
			for _, res := range r.Resources {
				res.Name, res.Description, res.Aliases = "", "", nil
			}
		}},
		{"wildcard", []string{"*"}, func(*pb.ListResourcesResponse) {}},
		{"empty", []string{}, func(*pb.ListResourcesResponse) {}},
		{"omitted", nil, func(*pb.ListResourcesResponse) {}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			want := listResponse()
			tc.want(want)
			seen, got, err := runUnary(t, newUnary(t, fieldmask.WithResourceReadMask()), listMethod, listRequest(tc.paths...), listResponse(), nil)
			require.NoError(t, err)
			require.NotNil(t, seen)
			require.True(t, proto.Equal(want, got.(proto.Message)), "got %v", got)
		})
	}
}

func TestResourceReadMask_InvalidRejectedBeforeHandler(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		method string
		paths  []string
	}{
		{"unknown field", listMethod, []string{"name", "no_such_field"}},
		{"envelope field", listMethod, []string{"next_page_token"}},
		{"envelope-relative path", listMethod, []string{"resources.*.name"}},
		{"wildcard with paths", listMethod, []string{"*", "name"}},
		{"malformed", listMethod, []string{"name..x"}},
		{"index", listMethod, []string{"aliases.0"}},
		{"get unknown field", getMethod, []string{"no_such_field"}},
		{"get element of repeated field", getMethod, []string{"display_name"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var req proto.Message = listRequest(tc.paths...)
			var resp proto.Message = listResponse()
			if tc.method == getMethod {
				req = getRequest(tc.paths...)
				resp = &pb.Resource{Name: "n"}
			}
			seen, got, err := runUnary(t, newUnary(t, fieldmask.WithResourceReadMask()), tc.method, req, resp, nil)
			requireMaskViolation(t, err, "read_mask")
			require.Nil(t, seen, "the handler must not run")
			require.Nil(t, got)
		})
	}
}

// Get keeps addressing the response, which is the resource, even when it has a single repeated message field.
func TestResourceReadMask_GetAddressesResource(t *testing.T) {
	t.Parallel()
	resp := &pb.Resource{Id: "1", Name: "kept", Description: "dropped", Aliases: []*pb.Profile{{DisplayName: "dropped"}}}
	_, got, err := runUnary(t, newUnary(t, fieldmask.WithResourceReadMask()), getMethod, getRequest("name"), resp, nil)
	require.NoError(t, err)
	require.True(t, proto.Equal(&pb.Resource{Name: "kept"}, got.(proto.Message)), "got %v", got)
}

// A collection response with several repeated message fields has no single resource field: the paths address it.
func TestResourceReadMask_AmbiguousCollectionAddressesResponse(t *testing.T) {
	t.Parallel()
	resp := &pb.SearchResourcesResponse{
		Resources:     []*pb.Resource{{Name: "dropped"}},
		Profiles:      []*pb.Profile{{DisplayName: "dropped"}},
		NextPageToken: "kept",
	}
	uni := newUnary(t, fieldmask.WithResourceReadMask())
	_, got, err := runUnary(t, uni, searchMethod, listRequest("next_page_token"), resp, nil)
	require.NoError(t, err)
	require.True(t, proto.Equal(&pb.SearchResourcesResponse{NextPageToken: "kept"}, got.(proto.Message)), "got %v", got)

	seen, _, err := runUnary(t, uni, searchMethod, listRequest("name"), resp, nil)
	requireMaskViolation(t, err, "read_mask")
	require.Nil(t, seen)
}

// A method missing from the registry is checked by its response after the handler runs.
func TestResourceReadMask_UnregisteredMethod(t *testing.T) {
	t.Parallel()
	uni := newUnary(t, fieldmask.WithResourceReadMask())
	want := listResponse()
	for _, res := range want.Resources {
		res.Id, res.Description, res.Aliases = "", "", nil
	}
	_, got, err := runUnary(t, uni, "/x.v1.X/ListResources", listRequest("name"), listResponse(), nil)
	require.NoError(t, err)
	require.True(t, proto.Equal(want, got.(proto.Message)), "got %v", got)

	seen, got, err := runUnary(t, uni, "/x.v1.X/ListResources", listRequest("no_such_field"), listResponse(), nil)
	requireMaskViolation(t, err, "read_mask")
	require.NotNil(t, seen, "without a descriptor the handler runs")
	require.Nil(t, got)
}

// The default mode is unchanged: List paths address the response message.
func TestResourceReadMask_DefaultAddressesResponse(t *testing.T) {
	t.Parallel()
	uni := newUnary(t)
	_, got, err := runUnary(t, uni, listMethod, listRequest("resources.*.name", "next_page_token"), listResponse(), nil)
	require.NoError(t, err)
	require.True(t, proto.Equal(&pb.ListResourcesResponse{
		Resources:     []*pb.Resource{{Name: "first"}, {Name: "second"}},
		NextPageToken: "next",
	}, got.(proto.Message)), "got %v", got)

	seen, _, err := runUnary(t, uni, listMethod, listRequest("name"), listResponse(), nil)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	require.NotNil(t, seen, "the default mode checks paths against the response after the handler")
	st := status.Convert(err)
	info := errorInfo(st)
	require.NotNil(t, info)
	require.Equal(t, "INVALID_FIELD_MASK", info.GetReason())
	require.Len(t, st.Details(), 1, "the default refusal names no field")

	seen, _, err = runUnary(t, uni, listMethod, listRequest("*", "name"), listResponse(), nil)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	require.Nil(t, seen, "a malformed mask is refused before the handler")
}

func TestResourceReadMask_MetadataHeader(t *testing.T) {
	t.Parallel()
	uni := newUnary(t, fieldmask.WithResourceReadMask())
	handler := func(context.Context, any) (any, error) { return listResponse(), nil }
	info := &grpc.UnaryServerInfo{FullMethod: listMethod}

	ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs(fieldmask.DefaultMetadataReadMaskHeader, "name"))
	got, err := uni(ctx, listRequest(), info, handler)
	require.NoError(t, err)
	for _, res := range got.(*pb.ListResourcesResponse).GetResources() {
		require.Empty(t, res.GetId())
		require.NotEmpty(t, res.GetName())
	}
	require.Equal(t, "next", got.(*pb.ListResourcesResponse).GetNextPageToken())

	ctx = metadata.NewIncomingContext(t.Context(), metadata.Pairs(fieldmask.DefaultMetadataReadMaskHeader, "next_page_token"))
	_, err = uni(ctx, listRequest(), info, handler)
	requireMaskViolation(t, err, "read_mask")
}

func TestResourceReadMask_ReadMaskFromContext(t *testing.T) {
	t.Parallel()
	var seen *fieldmaskpb.FieldMask
	uni := newUnary(t, fieldmask.WithResourceReadMask())
	_, err := uni(t.Context(), listRequest("name"), &grpc.UnaryServerInfo{FullMethod: listMethod}, func(ctx context.Context, _ any) (any, error) {
		seen, _ = fieldmask.ReadMaskFromContext(ctx)
		return listResponse(), nil
	})
	require.NoError(t, err)
	require.Equal(t, []string{"name"}, seen.GetPaths(), "the handler sees the resource-relative paths")
}

func TestResourceReadMask_Stream(t *testing.T) {
	t.Parallel()
	stream := fieldmask.ServerInterceptor(fieldmask.WithResourceReadMask()).ServerStreamInterceptor()
	info := &grpc.StreamServerInfo{FullMethod: pagesMethod, IsServerStream: true}

	base := &maskStream{ctx: t.Context(), recv: listRequest("name")}
	var ss grpc.ServerStream = interceptors.NewServerWrappedStream(t.Context(), base, nil)
	resp := listResponse()
	require.NoError(t, stream(nil, ss, info, func(_ any, s grpc.ServerStream) error {
		var req pb.ListResourcesRequest
		if err := s.RecvMsg(&req); err != nil {
			return err
		}
		if err := s.SendMsg(resp); err != nil {
			return err
		}
		return s.SendMsg(resp)
	}))
	require.Len(t, base.sent, 2)
	for _, sent := range base.sent {
		page := sent.(*pb.ListResourcesResponse)
		require.Equal(t, "next", page.GetNextPageToken())
		require.Equal(t, int32(2), page.GetTotalSize())
		require.Equal(t, []string{"zone-b"}, page.GetUnreachable())
		require.Len(t, page.GetResources(), 2)
		for _, res := range page.GetResources() {
			require.Empty(t, res.GetId())
			require.NotEmpty(t, res.GetName())
		}
	}
	require.Equal(t, "one", resp.GetResources()[0].GetDescription(), "the handler's message must not be mutated")

	base = &maskStream{ctx: t.Context(), recv: listRequest("next_page_token")}
	ss = interceptors.NewServerWrappedStream(t.Context(), base, nil)
	err := stream(nil, ss, info, func(_ any, s grpc.ServerStream) error {
		var req pb.ListResourcesRequest
		return s.RecvMsg(&req)
	})
	requireMaskViolation(t, err, "read_mask")
}

func errorInfo(st *status.Status) *errdetails.ErrorInfo {
	for _, d := range st.Details() {
		if info, ok := d.(*errdetails.ErrorInfo); ok {
			return info
		}
	}
	return nil
}
