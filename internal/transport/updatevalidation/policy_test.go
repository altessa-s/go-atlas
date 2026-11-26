// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package updatevalidation

import (
	"errors"
	"testing"

	"github.com/altessa-s/go-aip/fieldbehavior"
	"github.com/altessa-s/go-aip/fieldmask/update"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	pb "github.com/altessa-s/go-atlas/proto/gen/maskvalidationtest/v1"
)

func TestPolicyClassifiers(t *testing.T) {
	t.Parallel()
	resource := &pb.Settings{ApiKey: "sent"}
	root := resource.ProtoReflect().Descriptor().Fields()
	general := root.ByName("general")
	timezone := general.Message().Fields().ByName("time_zone")
	routes := root.ByName("notifications").Message().Fields().ByName("routes")
	kind := routes.Message().Fields().ByName("kind")
	delegates := root.ByName("delegates")
	nested := delegates.MapValue().Message().Fields().ByName("nested")
	apiKey, id, routing := root.ByName("api_key"), root.ByName("id"), root.ByName("routing_key")
	timestamp, note, uuid := root.ByName("update_time"), root.ByName("server_note"), root.ByName("format_uuid")

	spec, err := update.NewSpec(resource,
		update.WithAllowedPaths("general"), update.WithIdentityFields("routing_key"),
		update.WithAtomicMessage(func(md protoreflect.MessageDescriptor) bool {
			return md == general.Message() || update.DefaultAtomicMessage(md)
		}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := spec.ResolveMask(resource, &fieldmaskpb.FieldMask{Paths: []string{"update_time"}}); err == nil {
		t.Fatal("expected allow-list to exclude update_time")
	}
	sel := Selection{Resource: resource, Spec: spec}
	for _, tc := range []struct {
		name                     string
		route                    []protoreflect.FieldDescriptor
		required, output, always bool
	}{
		{name: "output timestamp excluded by allow-list", route: []protoreflect.FieldDescriptor{timestamp}, output: true},
		{name: "output scalar excluded by allow-list", route: []protoreflect.FieldDescriptor{note}, output: true},
		{name: "atomic child remains required", route: []protoreflect.FieldDescriptor{general, timezone}, required: true},
		{name: "list element", route: []protoreflect.FieldDescriptor{root.ByName("notifications"), routes, kind}, required: true},
		{name: "map element", route: []protoreflect.FieldDescriptor{delegates, nested, timezone}, required: true},
		{name: "named identity", route: []protoreflect.FieldDescriptor{routing}, always: true},
		{name: "default identity replaced", route: []protoreflect.FieldDescriptor{id}},
		{name: "supplied write-only field", route: []protoreflect.FieldDescriptor{apiKey}, always: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			policy, err := sel.FieldPolicy(tc.route...)
			if err != nil {
				t.Fatal(err)
			}
			if policy.Required != tc.required || policy.OutputOnly != tc.output {
				t.Errorf("FieldPolicy = %+v, want Required = %v, OutputOnly = %v", policy, tc.required, tc.output)
			}
			if len(tc.route) == 1 {
				if got, err := sel.AlwaysCheck(tc.route[0]); got != tc.always || err != nil {
					t.Errorf("AlwaysCheck = %v, %v, want %v, nil", got, err, tc.always)
				}
			}
		})
	}
	resource.ApiKey = ""
	if got, err := sel.AlwaysCheck(apiKey); got || err != nil {
		t.Errorf("AlwaysCheck for omitted write-only value = %v, %v, want false, nil", got, err)
	}

	resolver := fieldbehavior.ResolverFunc(func(fd protoreflect.FieldDescriptor) []annotations.FieldBehavior {
		switch fd {
		case timezone, note:
			return nil
		case uuid:
			return []annotations.FieldBehavior{annotations.FieldBehavior_OUTPUT_ONLY}
		default:
			return (fieldbehavior.AnnotationResolver{}).Resolve(fd)
		}
	})
	override, err := update.NewSpec(resource, update.WithAllowedPaths("general"), update.WithResolver(resolver))
	if err != nil {
		t.Fatal(err)
	}
	sel.Spec = override
	if policy, err := sel.FieldPolicy(delegates, nested, timezone); policy.Required || err != nil {
		t.Errorf("FieldPolicy = %+v, %v, want REQUIRED removed", policy, err)
	}
	for fd, want := range map[protoreflect.FieldDescriptor]bool{note: false, uuid: true} {
		if policy, err := sel.FieldPolicy(fd); policy.OutputOnly != want || err != nil {
			t.Errorf("FieldPolicy(%s) = %+v, %v, want OutputOnly = %v", fd.Name(), policy, err, want)
		}
	}
}

func TestStripOutputOnlyForeignChild(t *testing.T) {
	t.Parallel()
	md := (&pb.Settings{}).ProtoReflect().Descriptor()
	fileProto := protodesc.ToFileDescriptorProto(md.ParentFile())
	foreign, err := protodesc.NewFile(fileProto, protoregistry.GlobalFiles)
	if err != nil {
		t.Fatal(err)
	}
	resource := dynamicpb.NewMessage(md)
	general := md.Fields().ByName("general")
	timezone := general.Message().Fields().ByName("time_zone")
	child := dynamicpb.NewMessage(foreign.Messages().ByName(general.Message().Name()))
	foreignTimezone := child.Descriptor().Fields().ByNumber(timezone.Number())
	child.Set(foreignTimezone, protoreflect.ValueOfString("UTC"))
	resource.Set(general, protoreflect.ValueOfMessage(child))
	spec, err := update.NewSpec(resource, update.WithResolver(fieldbehavior.ResolverFunc(func(fd protoreflect.FieldDescriptor) []annotations.FieldBehavior {
		if fd == timezone {
			return []annotations.FieldBehavior{annotations.FieldBehavior_OUTPUT_ONLY}
		}
		return nil
	})))
	if err != nil {
		t.Fatal(err)
	}
	sel := Selection{Resource: resource, Spec: spec}
	if err := sel.StripOutputOnly(); err != nil {
		t.Fatal(err)
	}
	if child.Has(foreignTimezone) {
		t.Error("output-only field remains on the foreign child")
	}

	for _, msg := range fileProto.MessageType {
		if msg.GetName() == string(general.Message().Name()) {
			msg.Field = append(msg.Field, &descriptorpb.FieldDescriptorProto{
				Name: new("extra"), Number: new(int32(100)),
				Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
			})
		}
	}
	incompatible, err := protodesc.NewFile(fileProto, protoregistry.GlobalFiles)
	if err != nil {
		t.Fatal(err)
	}
	child = dynamicpb.NewMessage(incompatible.Messages().ByName(general.Message().Name()))
	child.Set(child.Descriptor().Fields().ByName("extra"), protoreflect.ValueOfString("unmapped"))
	resource.Set(general, protoreflect.ValueOfMessage(child))
	if err := sel.StripOutputOnly(); !errors.Is(err, update.ErrMessageMismatch) {
		t.Errorf("StripOutputOnly = %v, want ErrMessageMismatch", err)
	}
}

func TestStripOutputOnlyExtension(t *testing.T) {
	t.Parallel()
	file, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Name: new("policy_extension.proto"), Package: new("policytest"), Syntax: new("proto2"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: new("Resource"),
			ExtensionRange: []*descriptorpb.DescriptorProto_ExtensionRange{{
				Start: new(int32(100)), End: new(int32(200)),
			}},
		}},
		Extension: []*descriptorpb.FieldDescriptorProto{{
			Name: new("server_note"), Number: new(int32(100)), Extendee: new(".policytest.Resource"),
			Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
		}, {
			Name: new("client_note"), Number: new(int32(101)), Extendee: new(".policytest.Resource"),
			Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
		}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	resource := dynamicpb.NewMessage(file.Messages().Get(0))
	output := dynamicpb.NewExtensionType(file.Extensions().Get(0)).TypeDescriptor()
	input := dynamicpb.NewExtensionType(file.Extensions().Get(1)).TypeDescriptor()
	resource.Set(output, protoreflect.ValueOfString("server value"))
	resource.Set(input, protoreflect.ValueOfString("client value"))
	spec, err := update.NewSpec(resource, update.WithResolver(fieldbehavior.ResolverFunc(func(fd protoreflect.FieldDescriptor) []annotations.FieldBehavior {
		if fd == output {
			return []annotations.FieldBehavior{annotations.FieldBehavior_OUTPUT_ONLY}
		}
		return nil
	})))
	if err != nil {
		t.Fatal(err)
	}
	sel := Selection{Resource: resource, Spec: spec}
	if err := sel.StripOutputOnly(); err != nil {
		t.Fatal(err)
	}
	if resource.Has(output) || resource.Get(input).String() != "client value" {
		t.Errorf("StripOutputOnly = %v, want only client_note", resource)
	}
}
