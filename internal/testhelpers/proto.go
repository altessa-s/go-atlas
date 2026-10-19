// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package testhelpers

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// ApplyLeafPaths copies the field each dot path names from src to dst, as a
// storage update by leaf paths would: intermediate messages of dst are created
// on the way, and a field unset in src is cleared in dst. Every segment must
// name a declared field of a singular message; anything else fails the test.
// Unlike fieldmask.FieldMask.Merge, which replaces whole top-level fields, it
// leaves the siblings of a nested path untouched.
func ApplyLeafPaths(tb testing.TB, dst, src proto.Message, paths []string) {
	tb.Helper()
	for _, path := range paths {
		d, s := dst.ProtoReflect(), src.ProtoReflect()
		parts := strings.Split(path, ".")
		for i, part := range parts {
			fd := d.Descriptor().Fields().ByName(protoreflect.Name(part))
			if fd == nil {
				tb.Fatalf("ApplyLeafPaths: %q: no field %q in %s", path, part, d.Descriptor().FullName())
			}
			if i == len(parts)-1 {
				if s.Has(fd) {
					d.Set(fd, s.Get(fd))
				} else {
					d.Clear(fd)
				}
				break
			}
			if fd.Kind() != protoreflect.MessageKind || fd.IsList() || fd.IsMap() {
				tb.Fatalf("ApplyLeafPaths: %q: %q is not a singular message field", path, part)
			}
			d, s = d.Mutable(fd).Message(), s.Get(fd).Message()
		}
	}
}
