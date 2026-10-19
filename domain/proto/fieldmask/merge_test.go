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
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

// merge runs FieldMask.Merge for paths.
func merge(dst, src proto.Message, paths ...string) error {
	return fieldmask.FromPaths(paths...).Merge(dst, src)
}

func TestMerge(t *testing.T) {
	t.Parallel()

	stored := func() *pb.Resource {
		return &pb.Resource{
			Id: "r1", Name: "old", TenantId: "t1", Description: "old desc",
			Profile: &pb.Profile{DisplayName: "old profile"}, Tags: []string{"a"},
		}
	}
	tests := []struct {
		name  string
		src   *pb.Resource
		paths []string
		want  *pb.Resource
	}{
		{
			name:  "explicit paths copy and clear",
			src:   &pb.Resource{Name: "new", Profile: &pb.Profile{DisplayName: "ignored"}},
			paths: []string{"name", "description"},
			want: &pb.Resource{
				Id: "r1", Name: "new", TenantId: "t1",
				Profile: &pb.Profile{DisplayName: "old profile"}, Tags: []string{"a"},
			},
		},
		{
			name:  "camel and snake spellings resolve to one field",
			src:   &pb.Resource{TenantId: "t2"},
			paths: []string{"tenantId", "tenant_id"},
			want: &pb.Resource{
				Id: "r1", Name: "old", TenantId: "t2", Description: "old desc",
				Profile: &pb.Profile{DisplayName: "old profile"}, Tags: []string{"a"},
			},
		},
		{
			name:  "wildcard replaces every field",
			src:   &pb.Resource{Name: "new"},
			paths: []string{"*"},
			want:  &pb.Resource{Name: "new"},
		},
		{
			name:  "message field replaced as a whole",
			src:   &pb.Resource{Profile: &pb.Profile{Secret: "s"}},
			paths: []string{"profile"},
			want: &pb.Resource{
				Id: "r1", Name: "old", TenantId: "t1", Description: "old desc",
				Profile: &pb.Profile{Secret: "s"}, Tags: []string{"a"},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dst := stored()
			require.NoError(t, merge(dst, tc.src, tc.paths...))
			require.True(t, proto.Equal(tc.want, dst), "got %v, want %v", dst, tc.want)
		})
	}

	t.Run("mask implied from the set fields of src", func(t *testing.T) {
		t.Parallel()
		dst := stored()
		src := &pb.Resource{Id: "other", Description: "new desc", Tags: []string{"b", "c"}}
		mask := fieldmask.FromSetFields[fieldmask.FieldMask](src, fieldmask.WithSkipFields("id"))
		require.NoError(t, mask.Merge(dst, src))
		want := &pb.Resource{
			Id: "r1", Name: "old", TenantId: "t1", Description: "new desc",
			Profile: &pb.Profile{DisplayName: "old profile"}, Tags: []string{"b", "c"},
		}
		require.True(t, proto.Equal(want, dst), "got %v", dst)
	})
}

// TestMergeEmptyMaskChangesNothing pins that an empty mask carries no hidden
// policy: a mask that named only a write-only field the caller applies
// itself leaves no field to copy, and the other populated fields of src
// must not leak into dst.
func TestMergeEmptyMaskChangesNothing(t *testing.T) {
	t.Parallel()

	stored := func() *pb.Resource { return &pb.Resource{Id: "r1", Name: "old", Description: "old desc"} }
	src := &pb.Resource{Name: "new", Description: "new desc", Password: "secret"}
	for name, mask := range map[string]fieldmask.FieldMask{"nil": nil, "empty": fieldmask.FromPaths()} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dst := stored()
			require.NoError(t, mask.Merge(dst, src))
			require.True(t, proto.Equal(stored(), dst), "got %v", dst)
		})
	}
}

func TestMergeDoesNotShareMemory(t *testing.T) {
	t.Parallel()

	src := &pb.Resource{Profile: &pb.Profile{DisplayName: "p"}, Tags: []string{"a"}, Labels: map[string]*pb.Profile{"k": {Id: "1"}}}
	dst := &pb.Resource{}
	require.NoError(t, merge(dst, src, "profile", "tags", "labels"))

	src.Profile.DisplayName = "changed"
	src.Tags[0] = "changed"
	src.Labels["k"].Id = "changed"
	require.Equal(t, "p", dst.GetProfile().GetDisplayName())
	require.Equal(t, []string{"a"}, dst.GetTags())
	require.Equal(t, "1", dst.GetLabels()["k"].GetId())
}

func TestMergeErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		dst     proto.Message
		src     proto.Message
		paths   []string
		wantErr error // nil: a *ValidationError
	}{
		{name: "unknown path", dst: &pb.Resource{}, src: &pb.Resource{}, paths: []string{"name", "missing"}},
		{name: "nested path", dst: &pb.Resource{}, src: &pb.Resource{}, paths: []string{"profile.display_name"}},
		{name: "map entry", dst: &pb.Resource{}, src: &pb.Resource{}, paths: []string{"labels.k"}},
		{name: "wildcard not alone", dst: &pb.Resource{}, src: &pb.Resource{}, paths: []string{"*", "name"}, wantErr: fieldmask.ErrWildcardNotAlone},
		{name: "different types", dst: &pb.Resource{}, src: &pb.Profile{}, wantErr: fieldmask.ErrMessageMismatch},
		{name: "nil dst", src: &pb.Resource{}, wantErr: fieldmask.ErrMessageMismatch},
		{name: "nil src", dst: &pb.Resource{}, wantErr: fieldmask.ErrMessageMismatch},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := merge(tc.dst, tc.src, tc.paths...)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			_, ok := errors.AsType[*fieldmask.ValidationError](err)
			require.True(t, ok, "err = %v, want a ValidationError", err)
		})
	}

	t.Run("dst unchanged on error", func(t *testing.T) {
		t.Parallel()
		dst := &pb.Resource{Name: "keep"}
		_, ok := errors.AsType[*fieldmask.ValidationError](merge(dst, &pb.Resource{}, "name", "missing"))
		require.True(t, ok)
		require.Equal(t, "keep", dst.GetName())
		_, ok = errors.AsType[*fieldmask.ValidationError](merge(dst, &pb.Resource{}, "name", "profile.secret"))
		require.True(t, ok)
		require.Equal(t, "keep", dst.GetName())
	})

	t.Run("a different message with the same shape is a mismatch", func(t *testing.T) {
		t.Parallel()
		require.ErrorIs(t, merge(&fieldmaskpb.FieldMask{}, &pb.Resource{}), fieldmask.ErrMessageMismatch)
	})
}

func TestMergeTypedNilMessages(t *testing.T) {
	t.Parallel()

	var nilRes *pb.Resource
	masks := map[string][]string{"explicit": {"name"}, "empty": nil, "wildcard": {"*"}}
	for name, mask := range masks {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, merge(&pb.Resource{Name: "keep"}, nilRes, mask...), fieldmask.ErrMessageMismatch, "typed-nil src")
			require.ErrorIs(t, merge(nilRes, &pb.Resource{Name: "x"}, mask...), fieldmask.ErrMessageMismatch, "typed-nil dst")
		})
	}
}

func TestMergeIndependentDescriptorsMismatch(t *testing.T) {
	t.Parallel()

	// Two descriptors of the same full name, built independently, as for
	// dynamic messages loaded twice.
	build := func() *dynamicpb.Message {
		fdp := protodesc.ToFileDescriptorProto(pb.File_fieldbehaviortest_v1_test_proto)
		fd, err := protodesc.NewFile(fdp, protoregistry.GlobalFiles)
		require.NoError(t, err)
		md := fd.Messages().ByName("Resource")
		require.NotNil(t, md)
		m := dynamicpb.NewMessage(md)
		m.Set(md.Fields().ByName("name"), protoreflect.ValueOfString("n"))
		return m
	}
	a, b := build(), build()
	require.Equal(t, a.Descriptor().FullName(), b.Descriptor().FullName())

	require.ErrorIs(t, merge(a, b, "name"), fieldmask.ErrMessageMismatch)
	require.ErrorIs(t, merge(a, &pb.Resource{Name: "x"}, "*"), fieldmask.ErrMessageMismatch)

	// The same descriptor merges.
	c := dynamicpb.NewMessage(a.Descriptor())
	require.NoError(t, merge(c, a, "name"))
	require.Equal(t, "n", c.Get(a.Descriptor().Fields().ByName("name")).String())
}

func TestMergeGeneratedAndDynamicMismatch(t *testing.T) {
	t.Parallel()

	md := (&pb.Resource{}).ProtoReflect().Descriptor()
	dynamic := func() *dynamicpb.Message {
		m := dynamicpb.NewMessage(md)
		require.NoError(t, protojson.Unmarshal([]byte(`{"name":"dyn","profile":{"displayName":"p"},"aliases":[{"id":"a"}],"labels":{"k":{"id":"l"}}}`), m))
		return m
	}
	generated := func() *pb.Resource {
		return &pb.Resource{Name: "gen", Profile: &pb.Profile{DisplayName: "g"}, Aliases: []*pb.Profile{{Id: "x"}}}
	}
	masks := map[string][]string{
		"scalar":         {"name"},
		"nested message": {"name", "profile"},
		"list":           {"name", "aliases"},
		"map":            {"name", "labels"},
		"wildcard":       {"*"},
	}
	for name, mask := range masks {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dst := generated()
			require.ErrorIs(t, merge(dst, dynamic(), mask...), fieldmask.ErrMessageMismatch, "dynamic into generated")
			require.True(t, proto.Equal(generated(), dst), "dst must be unchanged, got %v", dst)

			ddst := dynamic()
			require.ErrorIs(t, merge(ddst, generated(), mask...), fieldmask.ErrMessageMismatch, "generated into dynamic")
			require.True(t, proto.Equal(dynamic(), ddst), "dst must be unchanged, got %v", ddst)
		})
	}

	t.Run("dynamic into dynamic of one descriptor merges", func(t *testing.T) {
		t.Parallel()
		dst := dynamicpb.NewMessage(md)
		require.NoError(t, merge(dst, dynamic(), "name", "profile", "aliases", "labels"))
		require.True(t, proto.Equal(dynamic(), dst), "got %v", dst)
	})
}
