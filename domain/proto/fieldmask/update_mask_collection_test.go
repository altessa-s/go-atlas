// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"errors"
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

// violationPaths returns the paths of the violations err carries, failing the
// test when err is not an *UpdateMaskBehaviorError.
func violationPaths(tb testing.TB, err error) []string {
	tb.Helper()
	behaviorErr, ok := errors.AsType[*fieldmask.UpdateMaskBehaviorError](err)
	require.True(tb, ok, "want *UpdateMaskBehaviorError, got %T (%v)", err, err)
	paths := make([]string, 0, len(behaviorErr.Violations))
	for _, v := range behaviorErr.Violations {
		paths = append(paths, v.Path)
	}
	return paths
}

// TestApplyUpdateMask_CollectionElementIdentifierRejected pins that a sub-path
// into collection elements cannot name an element identifier: through a map
// key, a map or list wildcard, or the implicit per-element form, the update
// would change the identifier of every addressed element. A list index is
// rejected before, as indexed access.
func TestApplyUpdateMask_CollectionElementIdentifierRejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
	}{
		{name: "keyed map", path: "labels.k.id"},
		{name: "quoted map key", path: "labels.`a.b`.id"},
		{name: "wildcard map", path: "labels.*.id"},
		{name: "repeated wildcard", path: "aliases.*.id"},
		{name: "implicit repeated", path: "aliases.id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			res := &pb.Resource{
				Aliases: []*pb.Profile{{Id: "client"}},
				Labels:  map[string]*pb.Profile{"k": {Id: "client"}, "a.b": {Id: "client"}},
			}
			err := fieldmask.FromPaths(tt.path, "description").ApplyUpdateMask(res)
			require.Equal(t, []string{tt.path}, violationPaths(t, err))

			behaviorErr, _ := errors.AsType[*fieldmask.UpdateMaskBehaviorError](err)
			require.Equal(t, annotations.FieldBehavior_IDENTIFIER, behaviorErr.Violations[0].Behavior)
		})
	}

	t.Run("list index", func(t *testing.T) {
		t.Parallel()

		err := fieldmask.FromPaths("aliases.0.id").ApplyUpdateMask(&pb.Resource{Aliases: []*pb.Profile{{Id: "client"}}})
		_, ok := errors.AsType[*fieldmask.ValidationError](err)
		require.True(t, ok, "got %v", err)
	})
}

// TestApplyUpdateMask_CollectionElementOutputOnlyRemoved pins that an
// OUTPUT_ONLY leaf under a collection element is removed from the mask, as for
// a singular sub-message, and a branch left empty disappears without keeping
// the element identifier; editable element fields stay, with the set element
// identifier added, and whole elements are accepted.
func TestApplyUpdateMask_CollectionElementOutputOnlyRemoved(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		paths []string
		want  []string
	}{
		{name: "keyed map", paths: []string{"labels.k.updated_at", "labels.k.display_name"}, want: []string{"labels.k.display_name", "labels.k.id"}},
		{name: "keyed map only output-only", paths: []string{"labels.k.updated_at"}, want: []string{}},
		{name: "implicit repeated", paths: []string{"aliases.updated_at", "aliases.display_name"}, want: []string{"aliases.display_name", "aliases.id"}},
		{name: "implicit repeated only output-only", paths: []string{"aliases.updated_at"}, want: []string{}},
		{name: "whole elements", paths: []string{"labels.k", "aliases.*"}, want: []string{"aliases.*", "labels.k"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			res := &pb.Resource{
				Aliases: []*pb.Profile{{Id: "a", DisplayName: "x", UpdatedAt: "client"}},
				Labels:  map[string]*pb.Profile{"k": {Id: "l", DisplayName: "x", UpdatedAt: "client"}},
			}
			mask := fieldmask.FromPaths(tt.paths...)
			require.NoError(t, mask.ApplyUpdateMask(res))
			require.ElementsMatch(t, tt.want, mask.ToPaths())
		})
	}
}

// elementDescriptor builds a resource whose collection elements nest
// messages and collections of their own:
//
//	message Inner { string id = 1 [IDENTIFIER]; string code = 2 [IMMUTABLE]; string note = 3; string stamp = 4 [OUTPUT_ONLY]; }
//	message Elem  { string name = 1; Inner inner = 2; map<string, Inner> subs = 3; repeated Inner items = 4;
//	                string eid = 5 [IDENTIFIER]; string ref = 6 [IDENTIFIER, OUTPUT_ONLY]; }
//	message Wrap  { string label = 1; Inner inner = 2; repeated Inner items = 3; }
//	message Res   { map<string, Elem> elems = 1; repeated Elem list = 2; string title = 3; map<int32, Elem> nums = 4;
//	                repeated Wrap wraps = 5; }
func elementDescriptor(tb testing.TB) protoreflect.MessageDescriptor {
	tb.Helper()

	behaviorOpts := func(b ...annotations.FieldBehavior) *descriptorpb.FieldOptions {
		o := &descriptorpb.FieldOptions{}
		proto.SetExtension(o, annotations.E_FieldBehavior, b)
		return o
	}
	msg := descriptorpb.FieldDescriptorProto_TYPE_MESSAGE
	str := descriptorpb.FieldDescriptorProto_TYPE_STRING
	opt := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	rep := descriptorpb.FieldDescriptorProto_LABEL_REPEATED
	field := func(name string, num int32, typ descriptorpb.FieldDescriptorProto_Type, label descriptorpb.FieldDescriptorProto_Label,
		typeName string, opts *descriptorpb.FieldOptions,
	) *descriptorpb.FieldDescriptorProto {
		f := &descriptorpb.FieldDescriptorProto{Name: new(name), Number: new(num), Type: &typ, Label: &label, Options: opts}
		if typeName != "" {
			f.TypeName = new(typeName)
		}
		return f
	}
	mapEntry := func(name string, keyType descriptorpb.FieldDescriptorProto_Type, valueType string) *descriptorpb.DescriptorProto {
		return &descriptorpb.DescriptorProto{
			Name:    new(name),
			Options: &descriptorpb.MessageOptions{MapEntry: new(true)},
			Field: []*descriptorpb.FieldDescriptorProto{
				field("key", 1, keyType, opt, "", nil),
				field("value", 2, msg, opt, valueType, nil),
			},
		}
	}

	fdp := &descriptorpb.FileDescriptorProto{
		Name:    new("fieldmask_element_test.proto"),
		Package: new("fieldmaskelemtest"),
		Syntax:  new("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: new("Inner"), Field: []*descriptorpb.FieldDescriptorProto{
				field("id", 1, str, opt, "", behaviorOpts(annotations.FieldBehavior_IDENTIFIER)),
				field("code", 2, str, opt, "", behaviorOpts(annotations.FieldBehavior_IMMUTABLE)),
				field("note", 3, str, opt, "", nil),
				field("stamp", 4, str, opt, "", behaviorOpts(annotations.FieldBehavior_OUTPUT_ONLY)),
			}},
			{Name: new("Wrap"), Field: []*descriptorpb.FieldDescriptorProto{
				field("label", 1, str, opt, "", nil),
				field("inner", 2, msg, opt, ".fieldmaskelemtest.Inner", nil),
				field("items", 3, msg, rep, ".fieldmaskelemtest.Inner", nil),
			}},
			{
				Name: new("Elem"),
				Field: []*descriptorpb.FieldDescriptorProto{
					field("name", 1, str, opt, "", nil),
					field("inner", 2, msg, opt, ".fieldmaskelemtest.Inner", nil),
					field("subs", 3, msg, rep, ".fieldmaskelemtest.Elem.SubsEntry", nil),
					field("items", 4, msg, rep, ".fieldmaskelemtest.Inner", nil),
					field("eid", 5, str, opt, "", behaviorOpts(annotations.FieldBehavior_IDENTIFIER)),
					field("ref", 6, str, opt, "", behaviorOpts(annotations.FieldBehavior_IDENTIFIER, annotations.FieldBehavior_OUTPUT_ONLY)),
				},
				NestedType: []*descriptorpb.DescriptorProto{mapEntry("SubsEntry", str, ".fieldmaskelemtest.Inner")},
			},
			{
				Name: new("Res"),
				Field: []*descriptorpb.FieldDescriptorProto{
					field("elems", 1, msg, rep, ".fieldmaskelemtest.Res.ElemsEntry", nil),
					field("list", 2, msg, rep, ".fieldmaskelemtest.Elem", nil),
					field("title", 3, str, opt, "", nil),
					field("nums", 4, msg, rep, ".fieldmaskelemtest.Res.NumsEntry", nil),
					field("wraps", 5, msg, rep, ".fieldmaskelemtest.Wrap", nil),
				},
				NestedType: []*descriptorpb.DescriptorProto{
					mapEntry("ElemsEntry", str, ".fieldmaskelemtest.Elem"),
					mapEntry("NumsEntry", descriptorpb.FieldDescriptorProto_TYPE_INT32, ".fieldmaskelemtest.Elem"),
				},
			},
		},
	}
	fd, err := protodesc.NewFile(fdp, nil)
	require.NoError(tb, err)
	return fd.Messages().ByName("Res")
}

// TestApplyUpdateMask_NestedInCollectionElement pins that the element schema
// check recurses: into singular messages and collections inside an element,
// with a singular message named whole inside an element rejected when it
// holds a server-managed field, while a collection inside an element can
// still be named whole.
func TestApplyUpdateMask_NestedInCollectionElement(t *testing.T) {
	t.Parallel()

	md := elementDescriptor(t)

	rejected := []struct {
		name  string
		paths []string
		want  []string
	}{
		{name: "identifier in element message", paths: []string{"elems.k.inner.id"}, want: []string{"elems.k.inner.id"}},
		{name: "immutable in element message", paths: []string{"list.*.inner.code"}, want: []string{"list.*.inner.code"}},
		{name: "element message named whole", paths: []string{"elems.*.inner"}, want: []string{"elems.*.inner.id", "elems.*.inner.code", "elems.*.inner.stamp"}},
		{name: "map in element", paths: []string{"elems.k.subs.x.id"}, want: []string{"elems.k.subs.x.id"}},
		{name: "list in element", paths: []string{"list.*.items.*.code", "list.items.id"}, want: []string{"list.items.id", "list.*.items.*.code"}},
	}
	for _, tt := range rejected {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := fieldmask.FromPaths(tt.paths...).ApplyUpdateMask(dynamicpb.NewMessage(md))
			require.Equal(t, tt.want, violationPaths(t, err))
		})
	}

	accepted := []struct {
		name  string
		paths []string
		want  []string
	}{
		{name: "editable leaves", paths: []string{"elems.k.inner.note", "list.*.items.*.note"}, want: []string{"elems.k.inner.note", "list.*.items.*.note"}},
		{name: "output-only removed", paths: []string{"elems.k.inner.stamp", "list.*.subs.x.stamp", "title"}, want: []string{"title"}},
		{name: "collection in element named whole", paths: []string{"elems.k.subs", "list.*.items"}, want: []string{"elems.k.subs", "list.*.items"}},
	}
	for _, tt := range accepted {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mask := fieldmask.FromPaths(tt.paths...)
			require.NoError(t, mask.ApplyUpdateMask(dynamicpb.NewMessage(md)))
			require.ElementsMatch(t, tt.want, mask.ToPaths())
		})
	}
}
