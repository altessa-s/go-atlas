// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldbehavior_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/fieldmask/update"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldbehavior"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/updatemask"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

func TestUpdate_NestedResourceSurvivesOutsideStrip(t *testing.T) {
	t.Parallel()

	// Reuse the annotated wrapper as an ordinary envelope inside the request.
	envelopeMD := wrapperDescriptor(t)
	files := new(protoregistry.Files)
	require.NoError(t, files.RegisterFile(envelopeMD.ParentFile()))

	fd, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Name: new("nested_resource_test.proto"), Package: new("nestedresource"), Syntax: new("proto3"),
		Dependency: []string{envelopeMD.ParentFile().Path()},
		MessageType: []*descriptorpb.DescriptorProto{{Name: new("Request"), Field: []*descriptorpb.FieldDescriptorProto{
			{Name: new("envelope"), Number: new(int32(1)), Type: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: new("." + string(envelopeMD.FullName()))},
		}}},
	}, files)
	require.NoError(t, err)

	req := dynamicpb.NewMessage(fd.Messages().Get(0))
	envelopeFD := req.Descriptor().Fields().ByName("envelope")
	envelope := req.Mutable(envelopeFD).Message()
	resourceFD := envelopeMD.Fields().ByName("resource")
	maskFD := envelopeMD.Fields().ByName("update_mask")
	revisionFD := envelopeMD.Fields().ByName("revision")
	resource := &pb.Resource{Id: "id-1", TenantId: "tenant-1", Description: "after"}
	envelope.Set(resourceFD, protoreflect.ValueOfMessage(resource.ProtoReflect()))
	envelope.Set(maskFD, protoreflect.ValueOfMessage((&fieldmaskpb.FieldMask{Paths: []string{"description"}}).ProtoReflect()))
	envelope.Set(revisionFD, protoreflect.ValueOfString("client"))
	extractor := updatemask.WithExtractor(func(_ context.Context, req proto.Message) (*fieldmaskpb.FieldMask, proto.Message, func(*fieldmaskpb.FieldMask), bool) {
		envelope := req.ProtoReflect().Get(envelopeFD).Message()
		mask := envelope.Get(maskFD).Message().Interface().(*fieldmaskpb.FieldMask)

		return mask, envelope.Get(resourceFD).Message().Interface(), func(mask *fieldmaskpb.FieldMask) {
			envelope.Set(maskFD, protoreflect.ValueOfMessage(mask.ProtoReflect()))
		}, true
	})
	uni := newUnary(t, fieldbehavior.WithUpdateNormalizer(updatemask.New(extractor)))
	stored := &pb.Resource{Id: "id-1", TenantId: "tenant-1", Description: "before", CreateTime: "server"}
	spec, err := update.NewSpec(stored)
	require.NoError(t, err)

	writes := 0
	patch := update.Patch[proto.Message, proto.Message, *int64]{
		Field: "envelope.resource", Spec: spec,
		Precondition: update.Int64Precondition[proto.Message]{Version: func(proto.Message) int64 { return 1 }},
		Read:         func(context.Context) (proto.Message, error) { return stored, nil },
		ToProto:      func(m proto.Message) proto.Message { return m },
		Save: func(_ context.Context, m proto.Message, _ update.Plan, _ *int64) (proto.Message, error) {
			stored = m.(*pb.Resource)
			writes++

			return m, nil
		},
	}
	_, err = uni(t.Context(), req, &grpc.UnaryServerInfo{FullMethod: "/x.v1.X/UpdateResource"}, func(ctx context.Context, r any) (any, error) {
		current := r.(*dynamicpb.Message).Get(envelopeFD).Message()
		require.False(t, current.Has(revisionFD))
		require.True(t, current.Has(resourceFD))

		got := current.Get(resourceFD).Message().Interface()
		require.Same(t, resource, got)

		_, ok := update.PlanFromContext(ctx, got)
		require.True(t, ok)

		mask := new(fieldmaskpb.FieldMask)
		proto.Merge(mask, current.Get(maskFD).Message().Interface())

		return patch.Run(ctx, got, "", mask)
	})
	require.NoError(t, err)
	require.Equal(t, 1, writes)
	require.True(t, proto.Equal(&pb.Resource{Id: "id-1", TenantId: "tenant-1", Description: "after", CreateTime: "server"}, stored), "stored %v", stored)
}
