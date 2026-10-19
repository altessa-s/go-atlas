// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/proto/fieldmask"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"

	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

// nodeDescriptor builds a self-referential message:
//
//	message Node { string id = 1 [IDENTIFIER]; Node child = 2; repeated Node kids = 3; string name = 4; }
func nodeDescriptor(tb testing.TB) protoreflect.MessageDescriptor {
	tb.Helper()

	idOpts := &descriptorpb.FieldOptions{}
	proto.SetExtension(idOpts, annotations.E_FieldBehavior,
		[]annotations.FieldBehavior{annotations.FieldBehavior_IDENTIFIER})

	msg := descriptorpb.FieldDescriptorProto_TYPE_MESSAGE
	str := descriptorpb.FieldDescriptorProto_TYPE_STRING
	opt := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	rep := descriptorpb.FieldDescriptorProto_LABEL_REPEATED

	fdp := &descriptorpb.FileDescriptorProto{
		Name:    new("fieldmask_identifier_test.proto"),
		Package: new("fieldmaskidtest"),
		Syntax:  new("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: new("Node"),
			Field: []*descriptorpb.FieldDescriptorProto{
				{Name: new("id"), Number: proto.Int32(1), Type: &str, Label: &opt, Options: idOpts},
				{Name: new("child"), Number: proto.Int32(2), Type: &msg, Label: &opt, TypeName: new(".fieldmaskidtest.Node")},
				{Name: new("kids"), Number: proto.Int32(3), Type: &msg, Label: &rep, TypeName: new(".fieldmaskidtest.Node")},
				{Name: new("name"), Number: proto.Int32(4), Type: &str, Label: &opt},
			},
		}},
	}
	fd, err := protodesc.NewFile(fdp, nil)
	require.NoError(tb, err)
	return fd.Messages().ByName("Node")
}

func TestSynthesisSkipFields_RecursiveMessage(t *testing.T) {
	t.Parallel()

	md := nodeDescriptor(t)
	fields := md.Fields()
	newNode := func(id string) *dynamicpb.Message {
		n := dynamicpb.NewMessage(md)
		n.Set(fields.ByName("id"), protoreflect.ValueOfString(id))
		return n
	}

	leaf := newNode("c2")
	mid := newNode("c1")
	mid.Set(fields.ByName("child"), protoreflect.ValueOfMessage(leaf))
	root := newNode("root")
	root.Set(fields.ByName("child"), protoreflect.ValueOfMessage(mid))
	kids := root.Mutable(fields.ByName("kids")).List()
	kids.Append(protoreflect.ValueOfMessage(newNode("k1")))

	// The walk follows the populated value, so a recursive type terminates
	// and every nested identifier on the chain is found; list elements are
	// not descended. The absent child of the last populated level is skipped
	// too: its schema holds an identifier.
	require.ElementsMatch(t, []string{"id", "child.id", "child.child.id", "child.child.child"}, fieldmask.SynthesisSkipFields(root))

	// An empty message yields its own identifier only: unset sub-messages
	// are not descended.
	require.Equal(t, []string{"id"}, fieldmask.SynthesisSkipFields(dynamicpb.NewMessage(md)))
}

func TestSynthesisSkipFields_NestedSingularOnly(t *testing.T) {
	t.Parallel()

	r := &pb.Resource{
		Id:      "id-1",
		Profile: &pb.Profile{Id: "p-1"},
		Aliases: []*pb.Profile{{Id: "a-1"}},
		Labels:  map[string]*pb.Profile{"k": {Id: "l-1"}},
	}
	// profile.updated_at is OUTPUT_ONLY: never written, so skipped too.
	require.ElementsMatch(t, []string{"id", "profile.id", "profile.updated_at"}, fieldmask.SynthesisSkipFields(r.ProtoReflect()))
}

// immutableDescriptor builds:
//
//	message Inner { string code = 1 [IMMUTABLE]; string note = 2; }
//	message Outer { Inner inner = 1; string name = 2; }
func immutableDescriptor(tb testing.TB) protoreflect.MessageDescriptor {
	tb.Helper()

	immOpts := &descriptorpb.FieldOptions{}
	proto.SetExtension(immOpts, annotations.E_FieldBehavior,
		[]annotations.FieldBehavior{annotations.FieldBehavior_IMMUTABLE})

	msg := descriptorpb.FieldDescriptorProto_TYPE_MESSAGE
	str := descriptorpb.FieldDescriptorProto_TYPE_STRING
	opt := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL

	fdp := &descriptorpb.FileDescriptorProto{
		Name:    new("fieldmask_immutable_test.proto"),
		Package: new("fieldmaskimmtest"),
		Syntax:  new("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: new("Inner"), Field: []*descriptorpb.FieldDescriptorProto{
				{Name: new("code"), Number: proto.Int32(1), Type: &str, Label: &opt, Options: immOpts},
				{Name: new("note"), Number: proto.Int32(2), Type: &str, Label: &opt},
			}},
			{Name: new("Outer"), Field: []*descriptorpb.FieldDescriptorProto{
				{Name: new("inner"), Number: proto.Int32(1), Type: &msg, Label: &opt, TypeName: new(".fieldmaskimmtest.Inner")},
				{Name: new("name"), Number: proto.Int32(2), Type: &str, Label: &opt},
			}},
		},
	}
	fd, err := protodesc.NewFile(fdp, nil)
	require.NoError(tb, err)
	return fd.Messages().ByName("Outer")
}

// TestSynthesisSkipFields_NestedImmutable pins that an unset IMMUTABLE field
// inside a populated sub-message is skipped, so the synthesized mask selects
// the sub-message's set fields rather than the whole sub-message — which
// ApplyUpdateMask rejects, as the replacement would clear the immutable
// field. A set nested IMMUTABLE field is not skipped: it stays in the mask and
// is rejected like a top-level one.
func TestSynthesisSkipFields_NestedImmutable(t *testing.T) {
	t.Parallel()

	md := immutableDescriptor(t)
	innerFD := md.Fields().ByName("inner")
	innerMD := innerFD.Message()

	outer := dynamicpb.NewMessage(md)
	inner := dynamicpb.NewMessage(innerMD)
	inner.Set(innerMD.Fields().ByName("note"), protoreflect.ValueOfString("n"))
	outer.Set(innerFD, protoreflect.ValueOfMessage(inner))
	require.Equal(t, []string{"inner.code"}, fieldmask.SynthesisSkipFields(outer))

	inner.Set(innerMD.Fields().ByName("code"), protoreflect.ValueOfString("c"))
	require.Empty(t, fieldmask.SynthesisSkipFields(outer))
}

// TestFromSetFieldsForUpdate pins the mask synthesized for an empty
// update_mask: identifiers and nested OUTPUT_ONLY fields are skipped, a
// populated sub-message is masked by its other set fields, and collections are
// named whole, so the mask passes ApplyUpdateMask.
func TestFromSetFieldsForUpdate(t *testing.T) {
	t.Parallel()

	res := &pb.Resource{
		Id:          "id-1",
		Description: "d",
		Profile:     &pb.Profile{Id: "p-1", DisplayName: "Ann", UpdatedAt: "client"},
		Aliases:     []*pb.Profile{{Id: "a-1"}},
		Labels:      map[string]*pb.Profile{"k": {Id: "l-1"}},
	}
	mask := fieldmask.FromSetFieldsForUpdate(res)
	require.ElementsMatch(t, []string{"aliases", "description", "labels", "profile.display_name"}, mask.ToPaths())
	require.NoError(t, mask.ApplyUpdateMask(res))

	require.Empty(t, fieldmask.FromSetFieldsForUpdate(nil))
	require.Empty(t, fieldmask.FromSetFieldsForUpdate(&pb.Resource{Id: "id-1"}))
}

// TestFieldMask_DropIdentifiers pins that DropIdentifiers removes IDENTIFIER
// entries at every depth — top level, singular sub-messages and collection
// elements under a key, a wildcard or the implicit list form — and the
// branches they leave empty.
func TestFieldMask_DropIdentifiers(t *testing.T) {
	t.Parallel()

	mask := fieldmask.FromPaths(
		"id", "description",
		"profile.id", "profile.display_name",
		"labels.k.id", "labels.k.display_name", "labels.j.id",
		"aliases.*.id", "aliases.id",
	)
	mask.DropIdentifiers(&pb.Resource{})
	require.ElementsMatch(t, []string{"description", "labels.k.display_name", "profile.display_name"}, mask.ToPaths())

	whole := fieldmask.FromPaths("labels", "aliases.*")
	whole.DropIdentifiers(&pb.Resource{})
	require.ElementsMatch(t, []string{"aliases.*", "labels"}, whole.ToPaths(), "whole collections and elements are kept")

	fieldmask.FromPaths("id").DropIdentifiers(nil)
}
