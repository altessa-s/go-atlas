// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/proto/fieldmask"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"

	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

// impliedPaths is the mask an update implies when its mask is empty: the
// populated top-level fields in declaration order, without skip.
func impliedPaths(msg proto.Message, skip ...string) []string {
	return fieldmask.FromSetFields[[]string](msg,
		fieldmask.WithTopLevelOnly(), fieldmask.WithDeclarationOrder(), fieldmask.WithSkipFields(skip...))
}

func TestFromSetFieldsImpliedUpdateMask(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  proto.Message
		skip []string
		want []string
	}{
		{
			name: "populated fields in declaration order",
			msg: &pb.Resource{
				Id: "r1", Description: "d", Name: "n", Profile: &pb.Profile{DisplayName: "p"}, Tags: []string{"t"},
				Labels: map[string]*pb.Profile{"k": {}},
			},
			skip: []string{"id"},
			want: []string{"name", "description", "profile", "labels", "tags"},
		},
		{
			name: "zero values are not populated",
			msg:  &pb.Resource{Name: "", Aliases: []*pb.Profile{}, Labels: map[string]*pb.Profile{}},
			want: []string{},
		},
		{
			name: "set oneof contributes its set field",
			msg:  &pb.Resource{Source: &pb.Resource_SourceToken{SourceToken: "s"}},
			want: []string{"source_token"},
		},
		{
			name: "only skipped fields set",
			msg:  &pb.Resource{Id: "r1", Slug: "s"},
			skip: []string{"id", "slug"},
			want: []string{},
		},
		{
			name: "skip compares declared names",
			msg:  &pb.Resource{TenantId: "t"},
			skip: []string{"tenantId"},
			want: []string{"tenant_id"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, impliedPaths(tc.msg, tc.skip...))
		})
	}

	t.Run("nil message", func(t *testing.T) {
		t.Parallel()
		require.Nil(t, impliedPaths(nil))
	})
}

func TestFromSetFieldsOptions(t *testing.T) {
	t.Parallel()

	msg := &pb.Resource{Id: "r1", Name: "n", Profile: &pb.Profile{DisplayName: "p", Secret: "s"}, Tags: []string{"a", "b"}}

	t.Run("default is every set path, sorted", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, []string{"id", "name", "profile", "profile.display_name", "profile.secret", "tags", "tags.0", "tags.1"},
			fieldmask.FromSetFields[[]string](msg))
	})

	t.Run("top level only", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, []string{"id", "name", "profile", "tags"}, fieldmask.FromSetFields[[]string](msg, fieldmask.WithTopLevelOnly()))
	})

	t.Run("skip drops a nested field, its subtree and the parent covering it", func(t *testing.T) {
		t.Parallel()
		got := fieldmask.FromSetFields[[]string](msg, fieldmask.WithSkipFields("profile.secret", "tags"))
		require.Equal(t, []string{"id", "name", "profile.display_name"}, got)
	})

	t.Run("declaration order keeps sub-paths sorted under their field", func(t *testing.T) {
		t.Parallel()
		got := fieldmask.FromSetFields[[]string](&pb.Resource{Tags: []string{"a"}, Name: "n", Profile: &pb.Profile{Secret: "s", DisplayName: "p"}},
			fieldmask.WithDeclarationOrder())
		require.Equal(t, []string{"name", "profile", "profile.display_name", "profile.secret", "tags", "tags.0"}, got)
	})

	t.Run("a FieldMask result holds the top-level fields", func(t *testing.T) {
		t.Parallel()
		got := fieldmask.FromSetFields[fieldmask.FieldMask](msg, fieldmask.WithSkipFields("id"))
		require.Equal(t, []string{"name", "profile", "tags"}, got.ToPaths())
	})
}

// TestFromSetFieldsNestedSkip pins that a nested skipped field never reaches
// the mask through a parent that covers it, in both result forms and through
// Filter and Merge.
func TestFromSetFieldsNestedSkip(t *testing.T) {
	t.Parallel()

	src := func() *pb.Resource {
		return &pb.Resource{Name: "n", Description: "d", Profile: &pb.Profile{DisplayName: "p", Secret: "s"}}
	}
	skip := fieldmask.WithSkipFields("profile.secret")

	t.Run("parent expanded into its other set fields", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, []string{"description", "name", "profile.display_name"}, fieldmask.FromSetFields[[]string](src(), skip))
		mask := fieldmask.FromSetFields[fieldmask.FieldMask](src(), skip)
		require.Equal(t, []string{"description", "name", "profile.display_name"}, mask.ToPaths())
	})

	t.Run("Filter keeps the siblings and drops the skipped field", func(t *testing.T) {
		t.Parallel()
		msg := src()
		fieldmask.FromSetFields[fieldmask.FieldMask](msg, skip).Filter(msg)
		require.True(t, proto.Equal(&pb.Resource{Name: "n", Description: "d", Profile: &pb.Profile{DisplayName: "p"}}, msg), "got %v", msg)
	})

	t.Run("Merge rejects the partial parent instead of copying it whole", func(t *testing.T) {
		t.Parallel()
		dst := &pb.Resource{Id: "r1", Profile: &pb.Profile{Secret: "kept"}}
		err := fieldmask.FromSetFields[fieldmask.FieldMask](src(), skip).Merge(dst, src())
		_, ok := errors.AsType[*fieldmask.ValidationError](err)
		require.True(t, ok, "err = %v", err)
		require.True(t, proto.Equal(&pb.Resource{Id: "r1", Profile: &pb.Profile{Secret: "kept"}}, dst), "dst must be unchanged, got %v", dst)
	})

	t.Run("top level only leaves the covering field out", func(t *testing.T) {
		t.Parallel()
		got := fieldmask.FromSetFields[[]string](src(), skip, fieldmask.WithTopLevelOnly(), fieldmask.WithDeclarationOrder())
		require.Equal(t, []string{"name", "description"}, got)

		dst := &pb.Resource{Id: "r1", Profile: &pb.Profile{Secret: "kept"}}
		mask := fieldmask.FromSetFields[fieldmask.FieldMask](src(), skip, fieldmask.WithTopLevelOnly())
		require.NoError(t, mask.Merge(dst, src()))
		require.True(t, proto.Equal(&pb.Resource{Id: "r1", Name: "n", Description: "d", Profile: &pb.Profile{Secret: "kept"}}, dst), "got %v", dst)
	})

	t.Run("a path through a repeated or map field leaves the field out", func(t *testing.T) {
		t.Parallel()
		msg := &pb.Resource{
			Name:    "n",
			Aliases: []*pb.Profile{{DisplayName: "a", Secret: "s"}},
			Labels:  map[string]*pb.Profile{"k": {DisplayName: "l", Secret: "s"}},
		}
		opts := fieldmask.WithSkipFields("aliases.secret", "labels.secret")
		require.Equal(t, []string{"name"}, fieldmask.FromSetFields[[]string](msg, opts))
		mask := fieldmask.FromSetFields[fieldmask.FieldMask](msg, opts)
		require.Equal(t, []string{"name"}, mask.ToPaths())
		dst := &pb.Resource{}
		require.NoError(t, mask.Merge(dst, msg))
		require.True(t, proto.Equal(&pb.Resource{Name: "n"}, dst), "got %v", dst)
	})

	t.Run("a skip naming no field drops nothing", func(t *testing.T) {
		t.Parallel()
		got := fieldmask.FromSetFields[fieldmask.FieldMask](src(), fieldmask.WithSkipFields("profile.missing", "name.sub"))
		require.Equal(t, []string{"description", "name", "profile"}, got.ToPaths())
	})
}

// threeLevels returns a dynamic message Outer{mid: Middle{in: Inner{a, b}, c}, d}
// with every field set.
func threeLevels(t *testing.T) *dynamicpb.Message {
	t.Helper()
	str := descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()
	msgT := descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum()
	opt := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()
	field := func(name string, n int32, typ *descriptorpb.FieldDescriptorProto_Type, typeName string) *descriptorpb.FieldDescriptorProto {
		f := &descriptorpb.FieldDescriptorProto{Name: proto.String(name), Number: proto.Int32(n), Type: typ, Label: opt}
		if typeName != "" {
			f.TypeName = proto.String(typeName)
		}
		return f
	}
	fdp := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("levels_test.proto"),
		Package: proto.String("fieldmasktest.levels"),
		Syntax:  proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: proto.String("Inner"), Field: []*descriptorpb.FieldDescriptorProto{field("a", 1, str, ""), field("b", 2, str, "")}},
			{Name: proto.String("Middle"), Field: []*descriptorpb.FieldDescriptorProto{
				field("in", 1, msgT, ".fieldmasktest.levels.Inner"), field("c", 2, str, ""),
			}},
			{Name: proto.String("Outer"), Field: []*descriptorpb.FieldDescriptorProto{
				field("mid", 1, msgT, ".fieldmasktest.levels.Middle"), field("d", 2, str, ""),
			}},
		},
	}
	fd, err := protodesc.NewFile(fdp, nil)
	require.NoError(t, err)
	m := dynamicpb.NewMessage(fd.Messages().ByName("Outer"))
	require.NoError(t, protojson.Unmarshal([]byte(`{"mid":{"in":{"a":"A","b":"B"},"c":"C"},"d":"D"}`), m))
	return m
}

func TestFromSetFieldsNestedSkipMultiLevel(t *testing.T) {
	t.Parallel()

	msg := threeLevels(t)
	skip := fieldmask.WithSkipFields("mid.in.b")
	require.Equal(t, []string{"d", "mid.c", "mid.in.a"}, fieldmask.FromSetFields[[]string](msg, skip))

	mask := fieldmask.FromSetFields[fieldmask.FieldMask](msg, skip)
	require.Equal(t, []string{"d", "mid.c", "mid.in.a"}, mask.ToPaths())
	src := proto.Clone(msg)
	mask.Filter(msg)
	got, err := protojson.Marshal(msg)
	require.NoError(t, err)
	require.JSONEq(t, `{"mid":{"in":{"a":"A"},"c":"C"},"d":"D"}`, string(got))

	_, ok := errors.AsType[*fieldmask.ValidationError](mask.Merge(dynamicpb.NewMessage(msg.Descriptor()), src))
	require.True(t, ok, "Merge replaces whole top-level fields and cannot exclude b")
}

// TestFromSetFieldsOnlySkippedFieldsSet pins that a mask computed with skips
// that comes out empty selects nothing through Select and Merge (Filter keeps
// its legacy "empty mask keeps everything" meaning).
func TestFromSetFieldsOnlySkippedFieldsSet(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		msg  func() *pb.Resource
		skip []string
	}{
		"nested skip under its only populated parent": {
			msg:  func() *pb.Resource { return &pb.Resource{Profile: &pb.Profile{Secret: "s"}} },
			skip: []string{"profile.secret"},
		},
		"conservative exclusion of a repeated field": {
			msg:  func() *pb.Resource { return &pb.Resource{Aliases: []*pb.Profile{{DisplayName: "a", Secret: "s"}}} },
			skip: []string{"aliases.secret"},
		},
		"conservative exclusion of a map field": {
			msg:  func() *pb.Resource { return &pb.Resource{Labels: map[string]*pb.Profile{"k": {Secret: "s"}}} },
			skip: []string{"labels.secret"},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			mask := fieldmask.FromSetFields[fieldmask.FieldMask](tc.msg(), fieldmask.WithSkipFields(tc.skip...))
			require.True(t, mask.IsEmpty())

			selected := tc.msg()
			mask.Select(selected)
			require.True(t, proto.Equal(&pb.Resource{}, selected), "Select must clear everything, got %v", selected)

			dst := &pb.Resource{Id: "r1", Profile: &pb.Profile{Secret: "kept"}}
			require.NoError(t, mask.Merge(dst, tc.msg()))
			require.True(t, proto.Equal(&pb.Resource{Id: "r1", Profile: &pb.Profile{Secret: "kept"}}, dst), "Merge must change nothing, got %v", dst)
		})
	}
}

func TestSelect(t *testing.T) {
	t.Parallel()

	msg := func() *pb.Resource {
		return &pb.Resource{Id: "r1", Name: "n", Profile: &pb.Profile{DisplayName: "p", Secret: "s"}, Tags: []string{"a"}}
	}

	t.Run("non-empty mask behaves like Filter", func(t *testing.T) {
		t.Parallel()
		mask := fieldmask.FromPaths("name", "profile.display_name")
		selected, filtered := msg(), msg()
		mask.Select(selected)
		mask.Filter(filtered)
		require.True(t, proto.Equal(filtered, selected), "got %v, want %v", selected, filtered)
		require.True(t, proto.Equal(&pb.Resource{Name: "n", Profile: &pb.Profile{DisplayName: "p"}}, selected), "got %v", selected)
	})

	t.Run("empty mask clears everything, Filter keeps everything", func(t *testing.T) {
		t.Parallel()
		selected, filtered := msg(), msg()
		fieldmask.FieldMask(nil).Select(selected)
		fieldmask.FromPaths().Filter(filtered)
		require.True(t, proto.Equal(&pb.Resource{}, selected), "got %v", selected)
		require.True(t, proto.Equal(msg(), filtered), "got %v", filtered)
	})

	t.Run("nil and typed-nil messages are ignored", func(t *testing.T) {
		t.Parallel()
		var typedNil *pb.Resource
		fieldmask.FromPaths().Select(nil)
		fieldmask.FromPaths().Select(typedNil)
		fieldmask.FromPaths("name").Select(typedNil)
	})
}
