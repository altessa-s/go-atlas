// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldmask"

	"google.golang.org/genproto/googleapis/api/annotations"
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

// BenchmarkServerInterceptor_EmptyUpdateMask_Nested measures the empty-mask
// synthesis path on a resource with a populated sub-message holding an
// identifier and an OUTPUT_ONLY field, and populated collections.
func BenchmarkServerInterceptor_EmptyUpdateMask_Nested(b *testing.B) {
	uni := fieldmask.ServerInterceptor(fieldmask.WithApplyEmptyUpdateMask()).ServerUnaryInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/x.v1.X/UpdateResource"}
	handler := func(_ context.Context, _ any) (any, error) { return &pb.Resource{}, nil }

	b.ReportAllocs()
	for b.Loop() {
		req := &pb.UpdateResourceRequest{
			Resource: &pb.Resource{
				Id:          "id-1",
				Description: "d",
				Profile:     &pb.Profile{Id: "p-1", DisplayName: "Ann", UpdatedAt: "t"},
				Aliases:     []*pb.Profile{{Id: "a-1"}},
				Labels:      map[string]*pb.Profile{"k": {Id: "l-1"}},
			},
			UpdateMask: &fieldmaskpb.FieldMask{},
		}
		_, _ = uni(b.Context(), req, info, handler)
	}
}

// recursiveRequestDescriptor builds an update request over a recursive
// resource:
//
//	message Node { string id = 1 [IDENTIFIER]; Node child = 2; string name = 3; }
//	message UpdateNodeRequest { Node resource = 1; google.protobuf.FieldMask update_mask = 2; }
func recursiveRequestDescriptor(tb testing.TB) protoreflect.MessageDescriptor {
	tb.Helper()

	idOpts := &descriptorpb.FieldOptions{}
	proto.SetExtension(idOpts, annotations.E_FieldBehavior, []annotations.FieldBehavior{annotations.FieldBehavior_IDENTIFIER})
	msg := descriptorpb.FieldDescriptorProto_TYPE_MESSAGE
	str := descriptorpb.FieldDescriptorProto_TYPE_STRING
	opt := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL

	fdp := &descriptorpb.FileDescriptorProto{
		Name:       new("fieldmask_recursive_bench.proto"),
		Package:    new("fieldmaskrecbench"),
		Syntax:     new("proto3"),
		Dependency: []string{"google/protobuf/field_mask.proto"},
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: new("Node"), Field: []*descriptorpb.FieldDescriptorProto{
				{Name: new("id"), Number: new(int32(1)), Type: &str, Label: &opt, Options: idOpts},
				{Name: new("child"), Number: new(int32(2)), Type: &msg, Label: &opt, TypeName: new(".fieldmaskrecbench.Node")},
				{Name: new("name"), Number: new(int32(3)), Type: &str, Label: &opt},
			}},
			{Name: new("UpdateNodeRequest"), Field: []*descriptorpb.FieldDescriptorProto{
				{Name: new("resource"), Number: new(int32(1)), Type: &msg, Label: &opt, TypeName: new(".fieldmaskrecbench.Node")},
				{Name: new("update_mask"), Number: new(int32(2)), Type: &msg, Label: &opt, TypeName: new(".google.protobuf.FieldMask")},
			}},
		},
	}
	fd, err := protodesc.NewFile(fdp, protoregistry.GlobalFiles)
	require.NoError(tb, err)
	return fd.Messages().ByName("UpdateNodeRequest")
}

// BenchmarkServerInterceptor_EmptyUpdateMask_Recursive measures the
// empty-mask synthesis path on a recursive resource populated four levels
// deep, every level with an identifier and an editable field. The request is
// rebuilt per iteration, since the interceptor rewrites it.
func BenchmarkServerInterceptor_EmptyUpdateMask_Recursive(b *testing.B) {
	reqMD := recursiveRequestDescriptor(b)
	resFD := reqMD.Fields().ByName("resource")
	nodeMD := resFD.Message()
	nf := nodeMD.Fields()

	newRequest := func() *dynamicpb.Message {
		var node *dynamicpb.Message
		for i := range 4 {
			n := dynamicpb.NewMessage(nodeMD)
			n.Set(nf.ByName("id"), protoreflect.ValueOfString("id"))
			n.Set(nf.ByName("name"), protoreflect.ValueOfString("name"))
			if i > 0 {
				n.Set(nf.ByName("child"), protoreflect.ValueOfMessage(node))
			}
			node = n
		}
		req := dynamicpb.NewMessage(reqMD)
		req.Set(resFD, protoreflect.ValueOfMessage(node))
		req.Set(reqMD.Fields().ByName("update_mask"), protoreflect.ValueOfMessage((&fieldmaskpb.FieldMask{}).ProtoReflect()))
		return req
	}

	uni := fieldmask.ServerInterceptor(fieldmask.WithApplyEmptyUpdateMask()).ServerUnaryInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/x.v1.X/UpdateNode"}
	var seen proto.Message
	handler := func(_ context.Context, r any) (any, error) {
		seen, _ = r.(proto.Message)
		return dynamicpb.NewMessage(nodeMD), nil
	}
	_, err := uni(b.Context(), newRequest(), info, handler)
	require.NoError(b, err)
	mask, _ := seen.ProtoReflect().Get(reqMD.Fields().ByName("update_mask")).Message().Interface().(*fieldmaskpb.FieldMask)
	require.ElementsMatch(b, []string{"name", "child.name", "child.child.name", "child.child.child.name"}, mask.GetPaths())

	b.ReportAllocs()
	for b.Loop() {
		_, _ = uni(b.Context(), newRequest(), info, handler)
	}
}
