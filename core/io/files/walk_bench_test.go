// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package files_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/altessa-s/go-atlas/core/io/files"
)

// makeBenchTree builds a fixture tree under b.TempDir with depth subdirectories
// and width entries per level (mix of regular files and one nested directory).
// The total number of regular files is approximately width^depth.
func makeBenchTree(b *testing.B, depth, width int) string {
	b.Helper()
	root := b.TempDir()
	var build func(dir string, level int)
	build = func(dir string, level int) {
		for i := range width {
			name := fmt.Sprintf("file_%d.so", i)
			if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
				b.Fatal(err)
			}
		}
		if level >= depth {
			return
		}
		sub := filepath.Join(dir, "sub")
		if err := os.Mkdir(sub, 0o755); err != nil {
			b.Fatal(err)
		}
		build(sub, level+1)
	}
	build(root, 0)
	return root
}

func BenchmarkWalk_Flat(b *testing.B) {
	root := makeBenchTree(b, 0, 64)
	b.ReportAllocs()
	for b.Loop() {
		var n int
		for entry, err := range files.Walk(root,
			files.WithExtensions(".so"),
			files.WithFileTypes(files.FileTypeRegular),
		) {
			if err != nil {
				b.Fatal(err)
			}
			_ = entry
			n++
		}
		if n != 64 {
			b.Fatalf("expected 64 entries, got %d", n)
		}
	}
}

func BenchmarkWalk_Recursive(b *testing.B) {
	root := makeBenchTree(b, 4, 16)
	b.ReportAllocs()
	for b.Loop() {
		var n int
		for entry, err := range files.Walk(root,
			files.WithRecursive(),
			files.WithExtensions(".so"),
			files.WithFileTypes(files.FileTypeRegular),
		) {
			if err != nil {
				b.Fatal(err)
			}
			_ = entry
			n++
		}
		if n == 0 {
			b.Fatal("expected non-zero entries")
		}
	}
}

func BenchmarkWalk_NoFilters(b *testing.B) {
	root := makeBenchTree(b, 4, 16)
	b.ReportAllocs()
	for b.Loop() {
		for entry, err := range files.Walk(root, files.WithRecursive()) {
			if err != nil {
				b.Fatal(err)
			}
			_ = entry
		}
	}
}
