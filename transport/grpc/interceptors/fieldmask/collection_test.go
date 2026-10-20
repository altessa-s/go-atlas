// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/proto-gen-go/io/altessa/badrequest/v1"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldmask"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

// collectionResource returns a resource whose collection elements carry client
// values in their IDENTIFIER and OUTPUT_ONLY fields.
func collectionResource() *pb.Resource {
	return &pb.Resource{
		Id:      "id-1",
		Aliases: []*pb.Profile{{Id: "client", DisplayName: "a", UpdatedAt: "client"}},
		Labels:  map[string]*pb.Profile{"k": {Id: "client", DisplayName: "l", UpdatedAt: "client"}},
	}
}

// TestServerInterceptor_UpdateMask_CollectionElementIdentifierRejected pins,
// through the interceptor chain, that an update_mask sub-path naming the
// identifier of collection elements — keyed map, map wildcard, repeated
// wildcard, implicit repeated form, or inside a map value — is rejected with
// InvalidArgument and a BadRequest violation, and a list index is rejected as
// indexed access, so the handler never gets a mask authorizing an element
// identifier change.
func TestServerInterceptor_UpdateMask_CollectionElementIdentifierRejected(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"labels.k.id", "labels.*.id", "aliases.*.id", "aliases.id"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			req := &pb.UpdateResourceRequest{
				Resource:   collectionResource(),
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{path, "description"}},
			}
			_, _, err := runUnary(t, newUnary(t), "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
			require.Equal(t, codes.InvalidArgument, status.Code(err), "got %v", err)

			st, _ := status.FromError(err)
			var paths []string
			for _, d := range st.Details() {
				if br, ok := d.(*badrequestv1.BadRequest); ok {
					for _, v := range br.GetFieldViolations() {
						paths = append(paths, v.GetFieldPath())
					}
				}
			}
			require.Equal(t, []string{path}, paths)
		})
	}

	t.Run("list index", func(t *testing.T) {
		t.Parallel()

		req := &pb.UpdateResourceRequest{
			Resource:   collectionResource(),
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"aliases.0.id"}},
		}
		_, _, err := runUnary(t, newUnary(t), "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
		require.Equal(t, codes.InvalidArgument, status.Code(err), "got %v", err)
	})
}

// TestServerInterceptor_UpdateMask_CollectionElementPaths pins the mask the
// handler gets for accepted collection sub-paths: OUTPUT_ONLY element leaves
// are removed (and a branch left empty disappears), editable leaves and whole
// collections or elements stay, and the filter clears the element fields the
// mask does not name.
func TestServerInterceptor_UpdateMask_CollectionElementPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		paths []string
		want  []string
	}{
		{name: "keyed map", paths: []string{"labels.k.display_name", "labels.k.updated_at"}, want: []string{"labels.k.display_name"}},
		{name: "repeated wildcard", paths: []string{"aliases.*.display_name", "aliases.*.updated_at"}, want: []string{"aliases.*.display_name"}},
		{name: "implicit repeated", paths: []string{"aliases.updated_at", "description"}, want: []string{"description"}},
		{name: "whole", paths: []string{"labels", "aliases.*"}, want: []string{"aliases.*", "labels"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := &pb.UpdateResourceRequest{
				Resource:   collectionResource(),
				UpdateMask: &fieldmaskpb.FieldMask{Paths: tt.paths},
			}
			seenReq, _, err := runUnary(t, newUnary(t), "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
			require.NoError(t, err)
			got, ok := seenReq.(*pb.UpdateResourceRequest)
			require.True(t, ok)
			require.ElementsMatch(t, tt.want, got.GetUpdateMask().GetPaths())
		})
	}

	req := &pb.UpdateResourceRequest{
		Resource:   collectionResource(),
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"labels.k.display_name"}},
	}
	seenReq, _, err := runUnary(t, newUnary(t), "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
	require.NoError(t, err)
	got, ok := seenReq.(*pb.UpdateResourceRequest)
	require.True(t, ok)
	label := got.GetResource().GetLabels()["k"]
	require.Equal(t, "l", label.GetDisplayName())
	require.Equal(t, "client", label.GetId(), "the identifier of an addressed element is kept")
	require.Empty(t, label.GetUpdatedAt())
	require.Equal(t, "id-1", got.GetResource().GetId(), "the resource identifier is kept")
}

// TestServerInterceptor_UpdateMask_CollectionElementIdentifierPreserved pins,
// through the interceptor chain, that a sub-path into collection elements —
// keyed map, map wildcard, repeated wildcard or implicit repeated form — keeps
// the identifier of every addressed element in the body, while the mask the
// handler gets names only the client's paths: the identifiers ApplyUpdateMask
// added to keep them are removed again.
func TestServerInterceptor_UpdateMask_CollectionElementIdentifierPreserved(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path        string
		labels      bool
		wantAliases bool
	}{
		{path: "labels.k.display_name", labels: true},
		{path: "labels.*.display_name", labels: true},
		{path: "aliases.*.display_name", wantAliases: true},
		{path: "aliases.display_name", wantAliases: true},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()

			req := &pb.UpdateResourceRequest{
				Resource:   collectionResource(),
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{tt.path}},
			}
			seenReq, _, err := runUnary(t, newUnary(t), "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
			require.NoError(t, err)
			got, ok := seenReq.(*pb.UpdateResourceRequest)
			require.True(t, ok)
			require.Equal(t, []string{tt.path}, got.GetUpdateMask().GetPaths(), "the handler's mask names no identifier")

			res := got.GetResource()
			require.Equal(t, "id-1", res.GetId())
			if tt.labels {
				require.Equal(t, "client", res.GetLabels()["k"].GetId())
				require.Empty(t, res.GetLabels()["k"].GetUpdatedAt())
				require.Empty(t, res.GetAliases(), "a collection the mask does not name is cleared")
			}
			if tt.wantAliases {
				require.Equal(t, "client", res.GetAliases()[0].GetId())
				require.Empty(t, res.GetAliases()[0].GetUpdatedAt())
				require.Empty(t, res.GetLabels(), "a collection the mask does not name is cleared")
			}
		})
	}
}

// TestServerInterceptor_EmptyUpdateMask_CollectionsNamedWhole pins that the
// mask synthesized for an empty update_mask names populated collections whole,
// as replacements, and so is accepted even when elements carry identifiers.
func TestServerInterceptor_EmptyUpdateMask_CollectionsNamedWhole(t *testing.T) {
	t.Parallel()

	req := &pb.UpdateResourceRequest{
		Resource:   collectionResource(),
		UpdateMask: &fieldmaskpb.FieldMask{},
	}
	seenReq, _, err := runUnary(t, newUnary(t, fieldmask.WithApplyEmptyUpdateMask()), "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
	require.NoError(t, err)
	got, ok := seenReq.(*pb.UpdateResourceRequest)
	require.True(t, ok)
	require.ElementsMatch(t, []string{"aliases", "labels"}, got.GetUpdateMask().GetPaths())
}
