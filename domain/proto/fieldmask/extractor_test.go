// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/proto/fieldmask"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

func TestNewUpdateExtractor(t *testing.T) {
	t.Parallel()

	t.Run("nested mask via typed getters", func(t *testing.T) {
		t.Parallel()

		extract := fieldmask.NewUpdateExtractor(
			func(r *pb.NestedUpdateResourceRequest) *fieldmaskpb.FieldMask {
				return r.GetOptions().GetUpdateMask()
			},
			func(r *pb.NestedUpdateResourceRequest) *pb.Resource { return r.GetResource() },
			func(r *pb.NestedUpdateResourceRequest, m *fieldmaskpb.FieldMask) {
				if r.Options == nil {
					r.Options = &pb.NestedUpdateResourceRequest_Options{}
				}
				r.Options.UpdateMask = m
			},
		)

		req := &pb.NestedUpdateResourceRequest{
			Resource: &pb.Resource{Name: "n"},
			Options: &pb.NestedUpdateResourceRequest_Options{
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
			},
		}

		mask, res, writeback, ok := extract(t.Context(), req)
		require.True(t, ok)
		require.Equal(t, []string{"name"}, mask.GetPaths())
		require.NotNil(t, res)
		require.NotNil(t, writeback)

		writeback(&fieldmaskpb.FieldMask{Paths: []string{"name", "description"}})
		require.Equal(t, []string{"name", "description"}, req.GetOptions().GetUpdateMask().GetPaths())
	})

	t.Run("type mismatch returns ok=false", func(t *testing.T) {
		t.Parallel()

		extract := fieldmask.NewUpdateExtractor(
			func(r *pb.NestedUpdateResourceRequest) *fieldmaskpb.FieldMask { return r.GetOptions().GetUpdateMask() },
			func(r *pb.NestedUpdateResourceRequest) *pb.Resource { return r.GetResource() },
			nil,
		)

		_, _, _, ok := extract(t.Context(), &pb.Resource{})
		require.False(t, ok)
	})

	t.Run("nil mask returns ok=false", func(t *testing.T) {
		t.Parallel()

		extract := fieldmask.NewUpdateExtractor(
			func(r *pb.NestedUpdateResourceRequest) *fieldmaskpb.FieldMask { return r.GetOptions().GetUpdateMask() },
			func(r *pb.NestedUpdateResourceRequest) *pb.Resource { return r.GetResource() },
			nil,
		)

		req := &pb.NestedUpdateResourceRequest{Resource: &pb.Resource{Name: "n"}}
		_, _, _, ok := extract(t.Context(), req)
		require.False(t, ok)
	})

	t.Run("typed-nil resource returns ok=false", func(t *testing.T) {
		t.Parallel()

		extract := fieldmask.NewUpdateExtractor(
			func(r *pb.NestedUpdateResourceRequest) *fieldmaskpb.FieldMask { return r.GetOptions().GetUpdateMask() },
			func(*pb.NestedUpdateResourceRequest) *pb.Resource { return nil },
			nil,
		)

		req := &pb.NestedUpdateResourceRequest{
			Options: &pb.NestedUpdateResourceRequest_Options{
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
			},
		}
		_, _, _, ok := extract(t.Context(), req)
		require.False(t, ok)
	})

	t.Run("nil setMask skips writeback", func(t *testing.T) {
		t.Parallel()

		extract := fieldmask.NewUpdateExtractor(
			func(r *pb.NestedUpdateResourceRequest) *fieldmaskpb.FieldMask { return r.GetOptions().GetUpdateMask() },
			func(r *pb.NestedUpdateResourceRequest) *pb.Resource { return r.GetResource() },
			nil,
		)

		req := &pb.NestedUpdateResourceRequest{
			Resource: &pb.Resource{Name: "n"},
			Options: &pb.NestedUpdateResourceRequest_Options{
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
			},
		}
		_, _, writeback, ok := extract(t.Context(), req)
		require.True(t, ok)
		require.Nil(t, writeback)
	})

	t.Run("nil getter returns ok=false", func(t *testing.T) {
		t.Parallel()

		var getMask func(*pb.NestedUpdateResourceRequest) *fieldmaskpb.FieldMask
		extract := fieldmask.NewUpdateExtractor(
			getMask,
			func(r *pb.NestedUpdateResourceRequest) *pb.Resource { return r.GetResource() },
			nil,
		)

		_, _, _, ok := extract(t.Context(), &pb.NestedUpdateResourceRequest{})
		require.False(t, ok)
	})
}

func TestNewReadExtractor(t *testing.T) {
	t.Parallel()

	t.Run("nested read mask", func(t *testing.T) {
		t.Parallel()

		extract := fieldmask.NewReadExtractor(
			func(r *pb.NestedGetResourceRequest) *fieldmaskpb.FieldMask {
				return r.GetOptions().GetReadMask()
			},
		)

		req := &pb.NestedGetResourceRequest{
			Name: "id-1",
			Options: &pb.NestedGetResourceRequest_Options{
				ReadMask: &fieldmaskpb.FieldMask{Paths: []string{"name", "description"}},
			},
		}

		mask, ok := extract(t.Context(), req)
		require.True(t, ok)
		require.Equal(t, []string{"name", "description"}, mask.GetPaths())
	})

	t.Run("type mismatch returns ok=false", func(t *testing.T) {
		t.Parallel()

		extract := fieldmask.NewReadExtractor(
			func(r *pb.NestedGetResourceRequest) *fieldmaskpb.FieldMask { return r.GetOptions().GetReadMask() },
		)

		_, ok := extract(t.Context(), &pb.Resource{})
		require.False(t, ok)
	})

	t.Run("absent mask returns ok=false", func(t *testing.T) {
		t.Parallel()

		extract := fieldmask.NewReadExtractor(
			func(r *pb.NestedGetResourceRequest) *fieldmaskpb.FieldMask { return r.GetOptions().GetReadMask() },
		)

		_, ok := extract(t.Context(), &pb.NestedGetResourceRequest{Name: "x"})
		require.False(t, ok)
	})
}

func TestDefaultUpdateExtractor(t *testing.T) {
	t.Parallel()

	extract := fieldmask.DefaultUpdateExtractor()

	t.Run("flat AIP-134 shape works", func(t *testing.T) {
		t.Parallel()

		req := &pb.UpdateResourceRequest{
			Resource:   &pb.Resource{Name: "n"},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
		}
		mask, res, writeback, ok := extract(t.Context(), req)
		require.True(t, ok)
		require.Equal(t, []string{"name"}, mask.GetPaths())
		require.NotNil(t, res)
		require.NotNil(t, writeback)

		writeback(&fieldmaskpb.FieldMask{Paths: []string{"name", "tenant_id"}})
		require.Equal(t, []string{"name", "tenant_id"}, req.GetUpdateMask().GetPaths())
	})

	t.Run("nested shape returns ok=false", func(t *testing.T) {
		t.Parallel()

		req := &pb.NestedUpdateResourceRequest{
			Resource: &pb.Resource{Name: "n"},
			Options: &pb.NestedUpdateResourceRequest_Options{
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
			},
		}
		_, _, _, ok := extract(t.Context(), req)
		require.False(t, ok, "default extractor only knows top-level update_mask")
	})
}

func TestDefaultReadExtractor(t *testing.T) {
	t.Parallel()

	extract := fieldmask.DefaultReadExtractor()

	req := &pb.GetResourceRequest{
		Name:     "id-1",
		ReadMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
	}
	mask, ok := extract(t.Context(), req)
	require.True(t, ok)
	require.Equal(t, []string{"name"}, mask.GetPaths())
}

func TestNewUpdateExtractor_ReturnsProtoMessage(t *testing.T) {
	t.Parallel()

	extract := fieldmask.NewUpdateExtractor(
		func(r *pb.NestedUpdateResourceRequest) *fieldmaskpb.FieldMask { return r.GetOptions().GetUpdateMask() },
		func(r *pb.NestedUpdateResourceRequest) *pb.Resource { return r.GetResource() },
		nil,
	)

	req := &pb.NestedUpdateResourceRequest{
		Resource: &pb.Resource{},
		Options: &pb.NestedUpdateResourceRequest_Options{
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
		},
	}

	_, res, _, ok := extract(t.Context(), req)
	require.True(t, ok)
	var _ proto.Message = res
}
