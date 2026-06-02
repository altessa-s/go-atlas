// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
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

func TestExtractUpdateMask(t *testing.T) {
	t.Parallel()

	t.Run("returns mask and resource", func(t *testing.T) {
		t.Parallel()

		req := &pb.UpdateResourceRequest{
			Resource:   &pb.Resource{Name: "n", TenantId: "t"},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name", "tenant_id"}},
		}

		mask, res, ok := fieldmask.ExtractUpdateMask(req)
		require.True(t, ok)
		require.Equal(t, []string{"name", "tenant_id"}, mask.GetPaths())

		r, isResource := res.(*pb.Resource)
		require.True(t, isResource)
		require.Equal(t, "n", r.GetName())
	})

	t.Run("missing mask returns false", func(t *testing.T) {
		t.Parallel()

		req := &pb.UpdateResourceRequest{
			Resource: &pb.Resource{Name: "n"},
		}

		_, _, ok := fieldmask.ExtractUpdateMask(req)
		require.False(t, ok)
	})

	t.Run("missing resource returns false", func(t *testing.T) {
		t.Parallel()

		req := &pb.UpdateResourceRequest{
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
		}

		_, _, ok := fieldmask.ExtractUpdateMask(req)
		require.False(t, ok)
	})

	t.Run("nil request returns false", func(t *testing.T) {
		t.Parallel()

		_, _, ok := fieldmask.ExtractUpdateMask(nil)
		require.False(t, ok)
	})

	t.Run("wrong message shape returns false", func(t *testing.T) {
		t.Parallel()

		_, _, ok := fieldmask.ExtractUpdateMask(&pb.Resource{Name: "n"})
		require.False(t, ok)
	})

	t.Run("override mask field name", func(t *testing.T) {
		t.Parallel()

		req := &pb.UpdateResourceRequest{
			Resource:   &pb.Resource{Name: "n"},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
		}
		_, _, ok := fieldmask.ExtractUpdateMask(req, fieldmask.WithMaskField("does_not_exist"))
		require.False(t, ok)
	})

	t.Run("override resource field name", func(t *testing.T) {
		t.Parallel()

		req := &pb.UpdateResourceRequest{
			Resource:   &pb.Resource{Name: "n"},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
		}
		_, res, ok := fieldmask.ExtractUpdateMask(req, fieldmask.WithResourceField("resource"))
		require.True(t, ok)
		_, isResource := res.(*pb.Resource)
		require.True(t, isResource)
	})

	t.Run("override missing resource field returns false", func(t *testing.T) {
		t.Parallel()

		req := &pb.UpdateResourceRequest{
			Resource:   &pb.Resource{Name: "n"},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
		}
		_, _, ok := fieldmask.ExtractUpdateMask(req, fieldmask.WithResourceField("does_not_exist"))
		require.False(t, ok)
	})
}

func TestExtractReadMask(t *testing.T) {
	t.Parallel()

	t.Run("returns mask", func(t *testing.T) {
		t.Parallel()

		req := &pb.GetResourceRequest{
			Name:     "id",
			ReadMask: &fieldmaskpb.FieldMask{Paths: []string{"name", "tenant_id"}},
		}

		mask, ok := fieldmask.ExtractReadMask(req)
		require.True(t, ok)
		require.Equal(t, []string{"name", "tenant_id"}, mask.GetPaths())
	})

	t.Run("missing mask returns false", func(t *testing.T) {
		t.Parallel()

		req := &pb.GetResourceRequest{Name: "id"}
		_, ok := fieldmask.ExtractReadMask(req)
		require.False(t, ok)
	})

	t.Run("nil request returns false", func(t *testing.T) {
		t.Parallel()

		_, ok := fieldmask.ExtractReadMask(nil)
		require.False(t, ok)
	})

	t.Run("wrong message shape returns false", func(t *testing.T) {
		t.Parallel()

		_, ok := fieldmask.ExtractReadMask(&pb.Resource{Name: "n"})
		require.False(t, ok)
	})
}

func TestSetUpdateMask(t *testing.T) {
	t.Parallel()

	t.Run("writes mask back to request", func(t *testing.T) {
		t.Parallel()

		req := &pb.UpdateResourceRequest{
			Resource:   &pb.Resource{Name: "n"},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name", "create_time"}},
		}

		cleaned := &fieldmaskpb.FieldMask{Paths: []string{"name"}}
		require.NoError(t, fieldmask.SetUpdateMask(req, cleaned))
		require.Equal(t, []string{"name"}, req.GetUpdateMask().GetPaths())
	})

	t.Run("nil mask clears the field", func(t *testing.T) {
		t.Parallel()

		req := &pb.UpdateResourceRequest{
			Resource:   &pb.Resource{Name: "n"},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
		}
		require.NoError(t, fieldmask.SetUpdateMask(req, nil))
		require.False(t, req.ProtoReflect().Has(
			req.ProtoReflect().Descriptor().Fields().ByName("update_mask"),
		))
	})

	t.Run("nil request returns ErrFieldNotSettable", func(t *testing.T) {
		t.Parallel()

		err := fieldmask.SetUpdateMask(nil, &fieldmaskpb.FieldMask{})
		require.ErrorIs(t, err, fieldmask.ErrFieldNotSettable)
	})

	t.Run("wrong message shape returns ErrFieldNotSettable", func(t *testing.T) {
		t.Parallel()

		err := fieldmask.SetUpdateMask(&pb.Resource{}, &fieldmaskpb.FieldMask{})
		require.ErrorIs(t, err, fieldmask.ErrFieldNotSettable)
	})

	t.Run("custom mask field name", func(t *testing.T) {
		t.Parallel()

		err := fieldmask.SetUpdateMask(
			&pb.UpdateResourceRequest{}, &fieldmaskpb.FieldMask{},
			fieldmask.WithMaskField("does_not_exist"),
		)
		require.ErrorIs(t, err, fieldmask.ErrFieldNotSettable)
	})
}

func TestExtractUpdateMaskReturnsProtoMessage(t *testing.T) {
	t.Parallel()

	req := &pb.UpdateResourceRequest{
		Resource:   &pb.Resource{},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
	}

	_, res, ok := fieldmask.ExtractUpdateMask(req)
	require.True(t, ok)
	var _ proto.Message = res
}
