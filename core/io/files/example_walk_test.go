// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package files_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/altessa-s/go-atlas/core/io/files"
)

// ExampleWalk demonstrates flat iteration over a directory, filtered by
// extension. Output is sorted for deterministic comparison.
func ExampleWalk() {
	root, err := os.MkdirTemp("", "files_walk_example")
	if err != nil {
		fmt.Println("setup:", err)
		return
	}
	defer os.RemoveAll(root)

	for _, name := range []string{"a.so", "b.txt", "c.so"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o644); err != nil {
			fmt.Println("setup:", err)
			return
		}
	}

	var got []string
	for entry, err := range files.Walk(root,
		files.WithExtensions(".so"),
		files.WithFileTypes(files.FileTypeRegular),
	) {
		if err != nil {
			fmt.Println("walk:", err)
			return
		}
		got = append(got, entry.Name())
	}
	slices.Sort(got)
	for _, name := range got {
		fmt.Println(name)
	}

	// Output:
	// a.so
	// c.so
}

// ExampleWalk_recursive demonstrates recursive iteration with depth limiting.
func ExampleWalk_recursive() {
	root, err := os.MkdirTemp("", "files_walk_example_recursive")
	if err != nil {
		fmt.Println("setup:", err)
		return
	}
	defer os.RemoveAll(root)

	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		fmt.Println("setup:", err)
		return
	}
	for _, p := range []string{"top.yaml", "sub/inner.yaml", "sub/skip.txt"} {
		if err := os.WriteFile(filepath.Join(root, p), []byte("x"), 0o644); err != nil {
			fmt.Println("setup:", err)
			return
		}
	}

	var got []string
	for entry, err := range files.Walk(root,
		files.WithRecursive(),
		files.WithExtensions(".yaml"),
		files.WithFileTypes(files.FileTypeRegular),
	) {
		if err != nil {
			fmt.Println("walk:", err)
			return
		}
		rel, _ := filepath.Rel(root, entry.Path)
		got = append(got, filepath.ToSlash(rel))
	}
	slices.Sort(got)
	for _, p := range got {
		fmt.Println(p)
	}

	// Output:
	// sub/inner.yaml
	// top.yaml
}
