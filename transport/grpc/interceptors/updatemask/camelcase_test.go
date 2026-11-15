// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package updatemask_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/fieldmask/update"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/updatemask"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

func TestNormalize_CamelCaseMask(t *testing.T) {
	t.Parallel()

	req := newCamelCaseUpdateRequest(t, "description", "create_time")
	result, err := updatemask.New().Normalize(t.Context(), req)
	require.NoError(t, err)
	require.True(t, result.Located)
	require.Equal(t, 1, result.Paths)
	require.Same(t, req.Get(req.Descriptor().Fields().ByName("resource")).Message().Interface(), result.Resource)

	got := req
	res := got.Get(got.Descriptor().Fields().ByName("resource")).Message()
	fields := res.Descriptor().Fields()
	require.Equal(t, "id-1", res.Get(fields.ByName("id")).String(), "IDENTIFIER kept")
	require.Equal(t, "changed", res.Get(fields.ByName("description")).String(), "masked field kept")
	require.False(t, res.Has(fields.ByName("tenant_id")), "unselected IMMUTABLE cleared")
	require.False(t, res.Has(fields.ByName("create_time")), "OUTPUT_ONLY cleared")
	require.False(t, res.Has(fields.ByName("name")), "unmasked plain field dropped")

	mask := got.Get(got.Descriptor().Fields().ByName("updateMask")).Message()
	paths := mask.Get(mask.Descriptor().Fields().ByName("paths")).List()
	require.Equal(t, 1, paths.Len(), "effective mask written back to updateMask")
	require.Equal(t, "description", paths.Get(0).String())
}

// newCamelCaseUpdateRequest builds a request whose mask is declared updateMask,
// not merely given that JSON name. A nil paths slice leaves the mask omitted.
func newCamelCaseUpdateRequest(t *testing.T, paths ...string) *dynamicpb.Message {
	t.Helper()

	behavior := func(b annotations.FieldBehavior) *descriptorpb.FieldOptions {
		opts := &descriptorpb.FieldOptions{}
		proto.SetExtension(opts, annotations.E_FieldBehavior, []annotations.FieldBehavior{b})
		return opts
	}
	field := func(name string, num int32, opts *descriptorpb.FieldOptions) *descriptorpb.FieldDescriptorProto {
		return &descriptorpb.FieldDescriptorProto{
			Name: new(name), Number: new(num),
			Type:  descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
			Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Options: opts,
		}
	}
	fd, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Name: new("updatemask_camelcase_test.proto"), Package: new("updatemaskcamelcase"), Syntax: new("proto3"),
		Dependency: []string{
			annotations.File_google_api_field_behavior_proto.Path(),
			fieldmaskpb.File_google_protobuf_field_mask_proto.Path(),
		},
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: new("Resource"), Field: []*descriptorpb.FieldDescriptorProto{
				field("id", 1, behavior(annotations.FieldBehavior_IDENTIFIER)),
				field("create_time", 2, behavior(annotations.FieldBehavior_OUTPUT_ONLY)),
				field("tenant_id", 3, behavior(annotations.FieldBehavior_IMMUTABLE)),
				field("description", 4, nil),
				field("name", 5, nil),
			}},
			{Name: new("UpdateResourceRequest"), Field: []*descriptorpb.FieldDescriptorProto{
				{
					Name: new("resource"), Number: new(int32(1)),
					Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
					Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					TypeName: new(".updatemaskcamelcase.Resource"),
				},
				{
					Name: new("updateMask"), Number: new(int32(2)),
					Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
					Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					TypeName: new(".google.protobuf.FieldMask"),
				},
			}},
		},
	}, protoregistry.GlobalFiles)
	require.NoError(t, err)

	req := dynamicpb.NewMessage(fd.Messages().ByName("UpdateResourceRequest"))
	res := req.Mutable(req.Descriptor().Fields().ByName("resource")).Message()
	for name, value := range map[protoreflect.Name]string{
		"id": "id-1", "create_time": "server-time", "tenant_id": "tenant-1", "description": "changed", "name": "unmasked",
	} {
		res.Set(res.Descriptor().Fields().ByName(name), protoreflect.ValueOfString(value))
	}
	if paths != nil {
		mask := req.Mutable(req.Descriptor().Fields().ByName("updateMask")).Message()
		list := mask.Mutable(mask.Descriptor().Fields().ByName("paths")).List()
		for _, path := range paths {
			list.Append(protoreflect.ValueOfString(path))
		}
	}
	return req
}

// Selected protected values are compared from captured input, even if the
// handler mutates the visible value before completion.
func TestCamelCase_SelectedImmutableCompletion(t *testing.T) {
	t.Parallel()
	for _, changed := range []bool{false, true} {
		name := "unchanged"
		if changed {
			name = "changed"
		}
		t.Run(name, func(t *testing.T) {
			req := newCamelCaseUpdateRequest(t, "tenant_id")
			resource := req.Get(req.Descriptor().Fields().ByName("resource")).Message()
			tenant := resource.Descriptor().Fields().ByName("tenant_id")
			stored := proto.Clone(resource.Interface())
			if changed {
				stored.ProtoReflect().Set(tenant, protoreflect.ValueOfString("different"))
			}
			check := func(plan *update.DeferredPlan) {
				require.True(t, resource.Has(tenant), "selected protected input retained")
				resource.Clear(tenant)
				mask, err := plan.Complete(stored)
				if changed {
					var behavior *update.BehaviorError
					require.ErrorAs(t, err, &behavior)
					require.Equal(t, "tenant_id", behavior.Violations[0].Path)
				} else {
					require.NoError(t, err)
					require.Empty(t, mask)
				}
			}
			result, err := updatemask.New().Normalize(t.Context(), req)
			require.NoError(t, err)
			check(result.Plan)
		})
	}
}
