// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"testing"

	"github.com/stretchr/testify/require"

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

// TestServerInterceptor_UpdateMask_CollectionElementIdentifierDeferred verifies the v0.1.5 contract.
// Protected collection paths are deferred; invalid list paths are rejected.
func TestServerInterceptor_UpdateMask_CollectionElementIdentifierDeferred(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"labels.k.id", "labels.*.id", "aliases.*.id"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			req := &pb.UpdateResourceRequest{
				Resource:   collectionResource(),
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{path, "description"}},
			}
			_, _, err := runUnary(t, newUnary(t), "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
			require.NoError(t, err)
			require.Equal(t, []string{"description"}, req.UpdateMask.Paths)
		})
	}

	for _, path := range []string{"aliases.0.id", "aliases.id", "aliases.display_name"} {
		t.Run("invalid "+path, func(t *testing.T) {
			t.Parallel()

			req := &pb.UpdateResourceRequest{
				Resource:   collectionResource(),
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{path}},
			}
			_, _, err := runUnary(t, newUnary(t), "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
			require.Equal(t, codes.InvalidArgument, status.Code(err), "got %v", err)
		})
	}
}

// TestServerInterceptor_UpdateMask_CollectionElementPaths verifies the v0.1.5 contract.
// Mutable collection paths remain writable; whole protected replacements are deferred.
func TestServerInterceptor_UpdateMask_CollectionElementPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		paths []string
		want  []string
	}{
		{name: "keyed map", paths: []string{"labels.k.display_name", "labels.k.updated_at"}, want: []string{"labels.k.display_name"}},
		{name: "repeated wildcard", paths: []string{"aliases.*.display_name", "aliases.*.updated_at"}, want: []string{"aliases.*.display_name"}},
		{name: "whole", paths: []string{"labels", "aliases.*"}, want: nil},
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

// TestServerInterceptor_UpdateMask_SelectedCollectionElementsPreserved verifies that
// selected elements keep their identities and positions while unselected collections are dropped.
func TestServerInterceptor_UpdateMask_SelectedCollectionElementsPreserved(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path      string
		labelKeys []string
	}{
		{path: "labels.k.display_name", labelKeys: []string{"k"}},
		{path: "labels.*.display_name", labelKeys: []string{"k", "other"}},
		// Numeric list indices are rejected by the mask grammar; use a wildcard
		// over distinct elements to verify that both positions survive.
		{path: "aliases.*.display_name"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()

			resource := collectionResource()
			resource.Labels["other"] = &pb.Profile{Id: "other-id", DisplayName: "other-label", UpdatedAt: "client"}
			resource.Aliases = append(resource.Aliases, &pb.Profile{Id: "second-id", DisplayName: "second-alias", UpdatedAt: "client"})
			req := &pb.UpdateResourceRequest{
				Resource:   resource,
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{tt.path}},
			}
			seenReq, _, err := runUnary(t, newUnary(t), "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
			require.NoError(t, err)
			got, ok := seenReq.(*pb.UpdateResourceRequest)
			require.True(t, ok)
			require.Equal(t, []string{tt.path}, got.GetUpdateMask().GetPaths(), "the handler's mask names no identifier")

			res := got.GetResource()
			require.Equal(t, "id-1", res.GetId())
			if len(tt.labelKeys) > 0 {
				require.Empty(t, res.GetAliases(), "the entirely unselected list is dropped")
				require.Len(t, res.GetLabels(), len(tt.labelKeys))
				want := map[string]*pb.Profile{
					"k":     {Id: "client", DisplayName: "l"},
					"other": {Id: "other-id", DisplayName: "other-label"},
				}
				for _, key := range tt.labelKeys {
					require.Contains(t, res.GetLabels(), key)
					label := res.GetLabels()[key]
					require.Equal(t, want[key].Id, label.GetId(), "key %s", key)
					require.Equal(t, want[key].DisplayName, label.GetDisplayName(), "key %s", key)
					require.Empty(t, label.GetUpdatedAt(), "key %s", key)
				}
			} else {
				require.Empty(t, res.GetLabels(), "the entirely unselected map is dropped")
				require.Len(t, res.GetAliases(), 2)
				for i, want := range []*pb.Profile{{Id: "client", DisplayName: "a"}, {Id: "second-id", DisplayName: "second-alias"}} {
					alias := res.GetAliases()[i]
					require.Equal(t, want.Id, alias.GetId(), "position %d", i)
					require.Equal(t, want.DisplayName, alias.GetDisplayName(), "position %d", i)
					require.Empty(t, alias.GetUpdatedAt(), "position %d", i)
				}
			}
		})
	}
}

// TestServerInterceptor_EmptyUpdateMask_CollectionsNoop verifies the v0.1.5 contract.
// The deprecated option cannot turn an empty collection mask into writes.
func TestServerInterceptor_EmptyUpdateMask_CollectionsNoop(t *testing.T) {
	t.Parallel()

	req := &pb.UpdateResourceRequest{
		Resource:   collectionResource(),
		UpdateMask: &fieldmaskpb.FieldMask{},
	}
	seenReq, _, err := runUnary(t, newUnary(t), "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
	require.NoError(t, err)
	got, ok := seenReq.(*pb.UpdateResourceRequest)
	require.True(t, ok)
	require.Empty(t, got.GetUpdateMask().GetPaths())
}
