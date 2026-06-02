// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"fmt"
	"testing"

	"github.com/altessa-s/go-atlas/domain/proto/fieldmask"
)

func BenchmarkFromPaths(b *testing.B) {
	paths := []string{
		"user.name",
		"user.email",
		"user.address.street",
		"user.address.city",
		"user.address.country",
		"meta.id",
		"meta.created_at",
	}

	b.ResetTimer()
	for b.Loop() {
		_ = fieldmask.FromPaths(paths...)
	}
}

// BenchmarkFromPaths_Backtick measures the parser hot path for AIP-161
// backtick-quoted map keys, where a single segment may contain dots
// and the splitter cannot fall back on strings.Split.
func BenchmarkFromPaths_Backtick(b *testing.B) {
	paths := []string{
		"reviews.`John Smith`",
		"reviews.`Alice O'Connor`.score",
		"metadata.`google.com/project`",
		"labels.`group.admin`.display_name",
	}

	b.ResetTimer()
	for b.Loop() {
		_ = fieldmask.FromPaths(paths...)
	}
}

func BenchmarkToPaths(b *testing.B) {
	m := fieldmask.FromPaths(
		"user.name",
		"user.email",
		"user.address.street",
		"user.address.city",
		"user.address.country",
		"meta.id",
		"meta.created_at",
	)

	b.ResetTimer()
	for b.Loop() {
		_ = m.ToPaths()
	}
}

func BenchmarkUnion(b *testing.B) {
	m1 := fieldmask.FromPaths("a.b", "a.c", "d")
	m2 := fieldmask.FromPaths("a.b.x", "e")

	b.ResetTimer()
	for b.Loop() {
		_ = m1.Union(m2)
	}
}

func BenchmarkIntersection(b *testing.B) {
	m1 := fieldmask.FromPaths("a.b", "a.c", "d")
	m2 := fieldmask.FromPaths("a", "d.e")

	b.ResetTimer()
	for b.Loop() {
		_ = m1.Intersection(m2)
	}
}

func BenchmarkLargeMask(b *testing.B) {
	var paths []string
	for i := range 100 {
		paths = append(paths, fmt.Sprintf("field_%d.sub_%d", i, i%10))
	}
	m := fieldmask.FromPaths(paths...)

	b.ResetTimer()
	for b.Loop() {
		_ = m.ToPaths()
	}
}
