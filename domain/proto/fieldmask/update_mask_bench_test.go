// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/proto/fieldmask"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"

	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

// BenchmarkApplyUpdateMask_HappyPath measures the steady-state cost of
// ApplyUpdateMask on a typical multi-path mask. The indexed-repeated check
// added for AIP-161 sits on this hot path and must stay cheap.
func BenchmarkApplyUpdateMask_HappyPath(b *testing.B) {
	paths := []string{"name", "description", "profile.display_name"}

	b.ReportAllocs()
	for b.Loop() {
		mask := fieldmask.FromPaths(paths...)
		_ = mask.ApplyUpdateMask(&pb.Resource{
			Name:        "n",
			Description: "d",
			Profile:     &pb.Profile{DisplayName: "p"},
		})
	}
}

// BenchmarkApplyUpdateMask_RejectIndexed measures the rejection path so we
// can spot regressions in the cost of producing the ValidationError.
func BenchmarkApplyUpdateMask_RejectIndexed(b *testing.B) {
	mask := fieldmask.FromPaths("aliases.0.display_name")
	msg := &pb.Resource{}

	b.ReportAllocs()
	for b.Loop() {
		_ = mask.ApplyUpdateMask(msg)
	}
}

// BenchmarkApplyUpdateMask_WithPathValidation measures the happy path with
// WithPathValidation enabled, so the extra Validate pass on a valid mask stays
// cheap relative to the unvalidated steady state.
func BenchmarkApplyUpdateMask_WithPathValidation(b *testing.B) {
	paths := []string{"name", "description", "profile.display_name"}

	b.ReportAllocs()
	for b.Loop() {
		mask := fieldmask.FromPaths(paths...)
		_ = mask.ApplyUpdateMask(&pb.Resource{
			Name:        "n",
			Description: "d",
			Profile:     &pb.Profile{DisplayName: "p"},
		}, fieldmask.WithPathValidation(fieldmask.PathValidationEnforce))
	}
}

// BenchmarkApplyUpdateMask_CollectionSubPaths measures a mask whose paths
// reach into collection elements, which ApplyUpdateMask checks against the
// element schema and strips of OUTPUT_ONLY leaves.
func BenchmarkApplyUpdateMask_CollectionSubPaths(b *testing.B) {
	paths := []string{"labels.k.display_name", "labels.k.updated_at", "aliases.*.display_name", "aliases.updated_at"}

	b.ReportAllocs()
	for b.Loop() {
		mask := fieldmask.FromPaths(paths...)
		_ = mask.ApplyUpdateMask(&pb.Resource{
			Aliases: []*pb.Profile{{Id: "a", DisplayName: "x"}},
			Labels:  map[string]*pb.Profile{"k": {Id: "l", DisplayName: "y"}},
		})
	}
}

// BenchmarkApplyUpdateMask_CollectionElementIdentifiers measures sub-paths
// that address many collection elements carrying set identifiers, which
// ApplyUpdateMask keeps in the body by visiting every addressed element. The
// resource is reused: after the first iteration the mask leaves it unchanged.
func BenchmarkApplyUpdateMask_CollectionElementIdentifiers(b *testing.B) {
	paths := []string{"labels.*.display_name", "aliases.*.display_name"}
	res := &pb.Resource{Labels: map[string]*pb.Profile{}}
	for i := range 16 {
		res.Aliases = append(res.Aliases, &pb.Profile{Id: "a", DisplayName: "x"})
		res.Labels[string(rune('a'+i))] = &pb.Profile{Id: "l", DisplayName: "y"}
	}

	b.ReportAllocs()
	for b.Loop() {
		mask := fieldmask.FromPaths(paths...)
		_ = mask.ApplyUpdateMask(res)
	}
}

// BenchmarkApplyUpdateMask_SingleMapKeyLargeMap measures a sub-path through
// one explicit key of a large map whose values carry set identifiers. The
// filter drops the other entries, so each iteration gets a fresh copy, made
// with the timer stopped.
func BenchmarkApplyUpdateMask_SingleMapKeyLargeMap(b *testing.B) {
	src := &pb.Resource{Labels: map[string]*pb.Profile{}}
	for i := range 256 {
		src.Labels["k"+strconv.Itoa(i)] = &pb.Profile{Id: "l", DisplayName: "y"}
	}

	b.ReportAllocs()
	for b.Loop() {
		b.StopTimer()
		res, _ := proto.Clone(src).(*pb.Resource)
		b.StartTimer()

		mask := fieldmask.FromPaths("labels.k7.display_name")
		_ = mask.ApplyUpdateMask(res)
	}
}

// BenchmarkApplyUpdateMask_ElementsWithoutIdentifiers measures sub-paths into
// collection elements whose type, and the types the sub-paths reach, declare
// no identifier: there is nothing to keep, so no element needs a visit. The
// resource is reused: after the first iteration the mask leaves it unchanged.
func BenchmarkApplyUpdateMask_ElementsWithoutIdentifiers(b *testing.B) {
	md := plainElementDescriptor(b)
	itemMD := md.Fields().ByName("items").Message()
	newItem := func() *dynamicpb.Message {
		child := dynamicpb.NewMessage(itemMD)
		child.Set(itemMD.Fields().ByName("note"), protoreflect.ValueOfString("c"))
		item := dynamicpb.NewMessage(itemMD)
		item.Set(itemMD.Fields().ByName("note"), protoreflect.ValueOfString("n"))
		item.Set(itemMD.Fields().ByName("child"), protoreflect.ValueOfMessage(child))
		return item
	}
	res := dynamicpb.NewMessage(md)
	items := res.Mutable(md.Fields().ByName("items")).List()
	byKey := res.Mutable(md.Fields().ByName("by_key")).Map()
	for i := range 16 {
		items.Append(protoreflect.ValueOfMessage(newItem()))
		byKey.Set(protoreflect.ValueOfString("k"+strconv.Itoa(i)).MapKey(), protoreflect.ValueOfMessage(newItem()))
	}
	paths := []string{"items.*.note", "items.*.child.note", "by_key.*.note"}

	b.ReportAllocs()
	for b.Loop() {
		mask := fieldmask.FromPaths(paths...)
		_ = mask.ApplyUpdateMask(res)
	}
}

// plainElementDescriptor builds a resource whose collection elements declare
// no field behaviors:
//
//	message Item { string note = 1; Item child = 2; }
//	message Res  { repeated Item items = 1; map<string, Item> by_key = 2; }
func plainElementDescriptor(tb testing.TB) protoreflect.MessageDescriptor {
	tb.Helper()

	msg := descriptorpb.FieldDescriptorProto_TYPE_MESSAGE
	str := descriptorpb.FieldDescriptorProto_TYPE_STRING
	opt := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	rep := descriptorpb.FieldDescriptorProto_LABEL_REPEATED
	field := func(name string, num int32, typ descriptorpb.FieldDescriptorProto_Type, label descriptorpb.FieldDescriptorProto_Label,
		typeName string,
	) *descriptorpb.FieldDescriptorProto {
		f := &descriptorpb.FieldDescriptorProto{Name: new(name), Number: new(num), Type: &typ, Label: &label}
		if typeName != "" {
			f.TypeName = new(typeName)
		}
		return f
	}

	fdp := &descriptorpb.FileDescriptorProto{
		Name:    new("fieldmask_plain_element_test.proto"),
		Package: new("fieldmaskplaintest"),
		Syntax:  new("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: new("Item"), Field: []*descriptorpb.FieldDescriptorProto{
				field("note", 1, str, opt, ""),
				field("child", 2, msg, opt, ".fieldmaskplaintest.Item"),
			}},
			{
				Name: new("Res"),
				Field: []*descriptorpb.FieldDescriptorProto{
					field("items", 1, msg, rep, ".fieldmaskplaintest.Item"),
					field("by_key", 2, msg, rep, ".fieldmaskplaintest.Res.ByKeyEntry"),
				},
				NestedType: []*descriptorpb.DescriptorProto{{
					Name:    new("ByKeyEntry"),
					Options: &descriptorpb.MessageOptions{MapEntry: new(true)},
					Field: []*descriptorpb.FieldDescriptorProto{
						field("key", 1, str, opt, ""),
						field("value", 2, msg, opt, ".fieldmaskplaintest.Item"),
					},
				}},
			},
		},
	}
	fd, err := protodesc.NewFile(fdp, nil)
	require.NoError(tb, err)
	return fd.Messages().ByName("Res")
}

// BenchmarkFromSetFieldsForUpdate measures the mask synthesized for an empty
// update_mask on a resource with a populated sub-message and collections.
func BenchmarkFromSetFieldsForUpdate(b *testing.B) {
	res := &pb.Resource{
		Id:          "id-1",
		Description: "d",
		Profile:     &pb.Profile{Id: "p-1", DisplayName: "Ann", UpdatedAt: "t"},
		Aliases:     []*pb.Profile{{Id: "a-1"}},
		Labels:      map[string]*pb.Profile{"k": {Id: "l-1"}},
	}

	b.ReportAllocs()
	for b.Loop() {
		_ = fieldmask.FromSetFieldsForUpdate(res)
	}
}
