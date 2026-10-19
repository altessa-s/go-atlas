// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/domain/proto/fieldmask"

	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

func BenchmarkFromSetFieldsImplied(b *testing.B) {
	msg := &pb.Resource{Id: "r1", Name: "n", Description: "d", Profile: &pb.Profile{DisplayName: "p"}}
	b.ReportAllocs()
	for b.Loop() {
		_ = fieldmask.FromSetFields[[]string](msg,
			fieldmask.WithTopLevelOnly(), fieldmask.WithDeclarationOrder(), fieldmask.WithSkipFields("id"))
	}
}

func BenchmarkResolve(b *testing.B) {
	md := (&pb.Resource{}).ProtoReflect().Descriptor()
	mask := fieldmask.FromPaths("tenantId", "profile.displayName", "aliases.*.id")
	b.ReportAllocs()
	for b.Loop() {
		if _, err := mask.Resolve(md); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMerge(b *testing.B) {
	src := &pb.Resource{Name: "new", Description: "d", Profile: &pb.Profile{DisplayName: "p"}}
	mask := fieldmask.FromPaths("name", "description", "profile")
	b.ReportAllocs()
	for b.Loop() {
		dst := &pb.Resource{Id: "r1", Name: "old"}
		if err := mask.Merge(dst, src); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSelect(b *testing.B) {
	mask := fieldmask.FromPaths("name", "profile.display_name")
	b.ReportAllocs()
	for b.Loop() {
		msg := &pb.Resource{Id: "r1", Name: "n", Profile: &pb.Profile{DisplayName: "p", Secret: "s"}}
		mask.Select(msg)
	}
}
