// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/proto/fieldmask"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

// TestApplyUpdateMask_CollectionElementIdentifierPreserved pins that a
// sub-path into collection elements keeps the set identifier of every
// addressed element in the body, as for a singular sub-message: through a map
// key, the "*" wildcard over maps and lists, and the implicit per-element
// list form. The identifier enters the mask so the filter keeps it, and
// DropIdentifiers removes it again. Elements the mask does not address are
// filtered as before, and whole elements or collections stay replacements.
func TestApplyUpdateMask_CollectionElementIdentifierPreserved(t *testing.T) {
	t.Parallel()

	newRes := func() *pb.Resource {
		return &pb.Resource{
			Aliases: []*pb.Profile{{Id: "a1", DisplayName: "x", UpdatedAt: "client"}, {DisplayName: "y"}},
			Labels: map[string]*pb.Profile{
				"k":   {Id: "l1", DisplayName: "x", UpdatedAt: "client"},
				"a.b": {Id: "l2", DisplayName: "y"},
			},
		}
	}

	tests := []struct {
		name    string
		paths   []string
		want    []string
		dropped []string
		check   func(t *testing.T, res *pb.Resource)
	}{
		{
			name:    "keyed map",
			paths:   []string{"labels.k.display_name"},
			want:    []string{"labels.k.display_name", "labels.k.id"},
			dropped: []string{"labels.k.display_name"},
			check: func(t *testing.T, res *pb.Resource) {
				t.Helper()
				require.Equal(t, "l1", res.GetLabels()["k"].GetId())
				require.Empty(t, res.GetLabels()["k"].GetUpdatedAt())
				require.NotContains(t, res.GetLabels(), "a.b", "an entry the mask does not address is dropped")
			},
		},
		{
			name:    "quoted map key",
			paths:   []string{"labels.`a.b`.display_name"},
			want:    []string{"labels.`a.b`.display_name", "labels.`a.b`.id"},
			dropped: []string{"labels.`a.b`.display_name"},
			check: func(t *testing.T, res *pb.Resource) {
				t.Helper()
				require.Equal(t, "l2", res.GetLabels()["a.b"].GetId())
			},
		},
		{
			name:    "map wildcard",
			paths:   []string{"labels.*.display_name"},
			want:    []string{"labels.*.display_name", "labels.*.id"},
			dropped: []string{"labels.*.display_name"},
			check: func(t *testing.T, res *pb.Resource) {
				t.Helper()
				require.Equal(t, "l1", res.GetLabels()["k"].GetId())
				require.Equal(t, "l2", res.GetLabels()["a.b"].GetId())
			},
		},
		{
			name:    "keyed and wildcard map",
			paths:   []string{"labels.k.updated_at", "labels.k.display_name", "labels.*.display_name"},
			want:    []string{"labels.*.display_name", "labels.*.id", "labels.k.display_name", "labels.k.id"},
			dropped: []string{"labels.*.display_name", "labels.k.display_name"},
			check: func(t *testing.T, res *pb.Resource) {
				t.Helper()
				require.Equal(t, "l1", res.GetLabels()["k"].GetId())
				require.Equal(t, "l2", res.GetLabels()["a.b"].GetId())
			},
		},
		{
			name:    "repeated wildcard",
			paths:   []string{"aliases.*.display_name"},
			want:    []string{"aliases.*.display_name", "aliases.*.id"},
			dropped: []string{"aliases.*.display_name"},
			check: func(t *testing.T, res *pb.Resource) {
				t.Helper()
				require.Equal(t, "a1", res.GetAliases()[0].GetId())
				require.Empty(t, res.GetAliases()[0].GetUpdatedAt())
				require.Empty(t, res.GetAliases()[1].GetId())
			},
		},
		{
			name:    "implicit repeated",
			paths:   []string{"aliases.display_name"},
			want:    []string{"aliases.display_name", "aliases.id"},
			dropped: []string{"aliases.display_name"},
			check: func(t *testing.T, res *pb.Resource) {
				t.Helper()
				require.Equal(t, "a1", res.GetAliases()[0].GetId())
			},
		},
		{
			name:    "whole elements and collections",
			paths:   []string{"labels.k", "aliases"},
			want:    []string{"aliases", "labels.k"},
			dropped: []string{"aliases", "labels.k"},
			check: func(t *testing.T, res *pb.Resource) {
				t.Helper()
				require.Equal(t, "l1", res.GetLabels()["k"].GetId(), "a whole element is kept as sent")
				require.Equal(t, "client", res.GetLabels()["k"].GetUpdatedAt(), "a whole element is kept as sent")
				require.Equal(t, "a1", res.GetAliases()[0].GetId())
			},
		},
		{
			name:    "whole list wildcard",
			paths:   []string{"aliases.*"},
			want:    []string{"aliases.*"},
			dropped: []string{"aliases.*"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			res := newRes()
			mask := fieldmask.FromPaths(tt.paths...)
			require.NoError(t, mask.ApplyUpdateMask(res))
			require.ElementsMatch(t, tt.want, mask.ToPaths())
			if tt.check != nil {
				tt.check(t, res)
			}

			mask.DropIdentifiers(res)
			require.ElementsMatch(t, tt.dropped, mask.ToPaths())
		})
	}

	t.Run("unset element identifiers", func(t *testing.T) {
		t.Parallel()

		res := &pb.Resource{
			Aliases: []*pb.Profile{{DisplayName: "x"}},
			Labels:  map[string]*pb.Profile{"k": {DisplayName: "y"}},
		}
		mask := fieldmask.FromPaths("aliases.*.display_name", "labels.k.display_name")
		require.NoError(t, mask.ApplyUpdateMask(res))
		require.ElementsMatch(t, []string{"aliases.*.display_name", "labels.k.display_name"}, mask.ToPaths())
	})

	t.Run("named element identifier still rejected", func(t *testing.T) {
		t.Parallel()

		res := newRes()
		err := fieldmask.FromPaths("labels.k.display_name", "labels.k.id").ApplyUpdateMask(res)
		require.Equal(t, []string{"labels.k.id"}, violationPaths(t, err))
	})
}

// TestApplyUpdateMask_NestedCollectionElementIdentifierPreserved pins that
// identifier preservation descends element-in-element: into singular
// messages, maps and lists inside an addressed element, at every level the
// mask branch reaches, while an IDENTIFIER that is also OUTPUT_ONLY is never
// preserved and keeps no client value.
func TestApplyUpdateMask_NestedCollectionElementIdentifierPreserved(t *testing.T) {
	t.Parallel()

	md := elementDescriptor(t)

	tests := []struct {
		name  string
		paths []string
		want  []string
		kept  map[string]string
	}{
		{
			name:  "map element",
			paths: []string{"elems.k.name"},
			want:  []string{"elems.k.name", "elems.k.eid"},
			kept:  map[string]string{"elems.k.eid": "e", "elems.k.ref": "", "elems.k.name": "n"},
		},
		{
			name:  "singular message in map element",
			paths: []string{"elems.k.inner.note"},
			want:  []string{"elems.k.inner.note", "elems.k.inner.id", "elems.k.eid"},
			kept:  map[string]string{"elems.k.eid": "e", "elems.k.inner.id": "i", "elems.k.inner.stamp": ""},
		},
		{
			name:  "map in map element",
			paths: []string{"elems.k.subs.x.note"},
			want:  []string{"elems.k.subs.x.note", "elems.k.subs.x.id", "elems.k.eid"},
			kept:  map[string]string{"elems.k.eid": "e", "elems.k.subs.x.id": "i"},
		},
		{
			name:  "list in list element",
			paths: []string{"list.*.items.*.note"},
			want:  []string{"list.*.items.*.note", "list.*.items.*.id", "list.*.eid"},
			kept:  map[string]string{"list.0.eid": "e", "list.0.ref": "", "list.0.items.0.id": "i", "list.0.items.0.note": "n"},
		},
		{
			name:  "implicit list in list element",
			paths: []string{"list.items.note"},
			want:  []string{"list.items.note", "list.items.id", "list.eid"},
			kept:  map[string]string{"list.0.eid": "e", "list.0.items.0.id": "i"},
		},
		{
			name:  "int map key",
			paths: []string{"nums.7.name"},
			want:  []string{"nums.7.name", "nums.7.eid"},
			kept:  map[string]string{"nums.7.eid": "e", "nums.7.ref": ""},
		},
		{
			name:  "non-canonical int map key matches no entry",
			paths: []string{"nums.07.name"},
			want:  []string{"nums.07.name"},
		},
		{
			name:  "absent map key",
			paths: []string{"elems.zz.name"},
			want:  []string{"elems.zz.name"},
		},
		{
			name:  "element without identifier, message below it",
			paths: []string{"wraps.*.inner.note"},
			want:  []string{"wraps.*.inner.note", "wraps.*.inner.id"},
			kept:  map[string]string{"wraps.0.inner.id": "i", "wraps.0.label": ""},
		},
		{
			name:  "element without identifier, list below it",
			paths: []string{"wraps.items.note"},
			want:  []string{"wraps.items.note", "wraps.items.id"},
			kept:  map[string]string{"wraps.0.items.0.id": "i"},
		},
		{
			name:  "element without identifier, nothing below it",
			paths: []string{"wraps.*.label"},
			want:  []string{"wraps.*.label"},
			kept:  map[string]string{"wraps.0.label": "l"},
		},
		{
			name:  "collection in element named whole",
			paths: []string{"elems.k.subs"},
			want:  []string{"elems.k.subs", "elems.k.eid"},
			kept:  map[string]string{"elems.k.eid": "e", "elems.k.subs.x.id": "i", "elems.k.subs.x.stamp": "s"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			res := newElementResource(t, md)
			mask := fieldmask.FromPaths(tt.paths...)
			require.NoError(t, mask.ApplyUpdateMask(res))
			require.ElementsMatch(t, tt.want, mask.ToPaths())
			for path, want := range tt.kept {
				require.Equal(t, want, stringAt(t, res, path), path)
			}

			mask.DropIdentifiers(res)
			require.ElementsMatch(t, tt.paths, mask.ToPaths())
		})
	}
}

// newElementResource returns a Res of [elementDescriptor] with one entry
// under "k" in elems and under 7 in nums and one list element, each an Elem
// whose fields are all set and whose inner, subs["x"] and items[0] are an
// Inner with all fields set, and one wraps element set the same way.
func newElementResource(tb testing.TB, md protoreflect.MessageDescriptor) *dynamicpb.Message {
	tb.Helper()

	elemMD := md.Fields().ByName("list").Message()
	innerMD := elemMD.Fields().ByName("inner").Message()
	set := func(m *dynamicpb.Message, name, v string) {
		m.Set(m.Descriptor().Fields().ByName(protoreflect.Name(name)), protoreflect.ValueOfString(v))
	}
	newInner := func() *dynamicpb.Message {
		m := dynamicpb.NewMessage(innerMD)
		set(m, "id", "i")
		set(m, "code", "c")
		set(m, "note", "n")
		set(m, "stamp", "s")
		return m
	}
	newElem := func() *dynamicpb.Message {
		m := dynamicpb.NewMessage(elemMD)
		set(m, "name", "n")
		set(m, "eid", "e")
		set(m, "ref", "r")
		fields := elemMD.Fields()
		m.Set(fields.ByName("inner"), protoreflect.ValueOfMessage(newInner()))
		m.Mutable(fields.ByName("subs")).Map().Set(protoreflect.ValueOfString("x").MapKey(), protoreflect.ValueOfMessage(newInner()))
		m.Mutable(fields.ByName("items")).List().Append(protoreflect.ValueOfMessage(newInner()))
		return m
	}

	res := dynamicpb.NewMessage(md)
	set(res, "title", "t")
	res.Mutable(md.Fields().ByName("elems")).Map().Set(protoreflect.ValueOfString("k").MapKey(), protoreflect.ValueOfMessage(newElem()))
	res.Mutable(md.Fields().ByName("list")).List().Append(protoreflect.ValueOfMessage(newElem()))
	res.Mutable(md.Fields().ByName("nums")).Map().Set(protoreflect.ValueOfInt32(7).MapKey(), protoreflect.ValueOfMessage(newElem()))

	wrapMD := md.Fields().ByName("wraps").Message()
	wrap := dynamicpb.NewMessage(wrapMD)
	set(wrap, "label", "l")
	wrap.Set(wrapMD.Fields().ByName("inner"), protoreflect.ValueOfMessage(newInner()))
	wrap.Mutable(wrapMD.Fields().ByName("items")).List().Append(protoreflect.ValueOfMessage(newInner()))
	res.Mutable(md.Fields().ByName("wraps")).List().Append(protoreflect.ValueOfMessage(wrap))
	return res
}

// stringAt returns the string field at the dot-separated path in msg, where a
// segment after a map field is a key (string or int32) and after a list field
// an index.
func stringAt(tb testing.TB, msg protoreflect.ProtoMessage, path string) string {
	tb.Helper()

	m := msg.ProtoReflect()
	segs := strings.Split(path, ".")
	for i := 0; i < len(segs); i++ {
		fd := m.Descriptor().Fields().ByName(protoreflect.Name(segs[i]))
		require.NotNil(tb, fd, path)
		v := m.Get(fd)
		switch {
		case fd.IsMap():
			i++
			key := protoreflect.ValueOfString(segs[i])
			if fd.MapKey().Kind() == protoreflect.Int32Kind {
				n, err := strconv.ParseInt(segs[i], 10, 32)
				require.NoError(tb, err)
				key = protoreflect.ValueOfInt32(int32(n))
			}
			v = v.Map().Get(key.MapKey())
			require.True(tb, v.IsValid(), "%s: no map entry %q", path, segs[i])
		case fd.IsList():
			i++
			idx, err := strconv.Atoi(segs[i])
			require.NoError(tb, err)
			require.Less(tb, idx, v.List().Len(), path)
			v = v.List().Get(idx)
		}
		if i == len(segs)-1 {
			return v.String()
		}
		m = v.Message()
	}
	return ""
}
