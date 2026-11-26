// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package updatevalidation

import (
	"reflect"
	"slices"
	"testing"

	"github.com/altessa-s/go-aip/fieldmask"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestMapKeyScope(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, key   string
		paths, want []string
		selected    bool
	}{
		{"overlapping descendants", "bob", []string{"*.nested.retention_days", "bob.nested.time_zone", "bob.kind"},
			[]string{"nested.retention_days", "nested.time_zone", "kind"}, true},
		{"whole wildcard", "bob", []string{"*", "bob.kind"}, nil, true},
		{"whole exact", "bob", []string{"*.kind", "bob"}, nil, true},
		{"unselected key", "alice", []string{"bob.kind"}, nil, false},
		{"literal star key", "*", []string{"*.kind", "`*`.nested"}, []string{"kind", "nested"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			paths := slices.Clone(tc.paths)
			for i, path := range paths {
				paths[i] = "delegates." + path
			}
			tree := fieldmask.FromPaths(paths...)["delegates"]
			original := tree.Clone()
			got, selected := MapKeyScope(tree, protoreflect.ValueOfString(tc.key).MapKey())
			var want fieldmask.FieldMask
			if tc.want != nil {
				want = fieldmask.FromPaths(tc.want...)
			}
			if selected != tc.selected || !reflect.DeepEqual(got, want) {
				t.Errorf("MapKeyScope = (%v, %v), want (%v, %v)", got, selected, want, tc.selected)
			}
			if !reflect.DeepEqual(tree, original) {
				t.Errorf("MapKeyScope changed input from %v to %v", original, tree)
			}
		})
	}
}
