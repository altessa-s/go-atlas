// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package files_test

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/altessa-s/go-atlas/core/io/files"
)

// collect drains a Walk iterator into name lists for assertion.
func collect(t *testing.T, root string, opts ...files.WalkOption) (names []string, errs []error) {
	t.Helper()
	for entry, err := range files.Walk(root, opts...) {
		if err != nil {
			errs = append(errs, err)
			continue
		}
		rel, relErr := filepath.Rel(root, entry.Path)
		if relErr != nil {
			t.Fatalf("rel(%q, %q): %v", root, entry.Path, relErr)
		}
		names = append(names, filepath.ToSlash(rel))
	}
	slices.Sort(names)
	return names, errs
}

// makeTree builds a fixture tree under root:
//
//	root/
//	  a.so
//	  b.SO
//	  notes.txt
//	  .hidden
//	  sub/
//	    c.so
//	    d.yaml
//	  empty/
func makeTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mustWrite := func(p string) {
		if err := os.WriteFile(filepath.Join(root, p), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustMkdir := func(p string) {
		if err := os.MkdirAll(filepath.Join(root, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("a.so")
	mustWrite("b.SO")
	mustWrite("notes.txt")
	mustWrite(".hidden")
	mustMkdir("sub")
	mustWrite("sub/c.so")
	mustWrite("sub/d.yaml")
	mustMkdir("empty")
	return root
}

func TestWalk_FlatDefault(t *testing.T) {
	root := makeTree(t)
	names, errs := collect(t, root)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	want := []string{".hidden", "a.so", "b.SO", "empty", "notes.txt", "sub"}
	if !slices.Equal(names, want) {
		t.Errorf("got %v, want %v", names, want)
	}
}

func TestWalk_ExtensionFilter_CaseInsensitive(t *testing.T) {
	root := makeTree(t)
	names, errs := collect(t, root, files.WithExtensions(".so"))
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	// Both a.so and b.SO should match (case-insensitive).
	want := []string{"a.so", "b.SO"}
	if !slices.Equal(names, want) {
		t.Errorf("got %v, want %v", names, want)
	}
}

func TestWalk_ExtensionFilter_NormalizesLeadingDot(t *testing.T) {
	root := makeTree(t)
	names, _ := collect(t, root, files.WithExtensions("so", "TXT"))
	want := []string{"a.so", "b.SO", "notes.txt"}
	if !slices.Equal(names, want) {
		t.Errorf("got %v, want %v", names, want)
	}
}

func TestWalk_FileTypes_RegularOnly(t *testing.T) {
	root := makeTree(t)
	names, _ := collect(t, root, files.WithFileTypes(files.FileTypeRegular))
	// Excludes "sub" and "empty" directories.
	want := []string{".hidden", "a.so", "b.SO", "notes.txt"}
	if !slices.Equal(names, want) {
		t.Errorf("got %v, want %v", names, want)
	}
}

func TestWalk_FileTypes_DirOnly(t *testing.T) {
	root := makeTree(t)
	names, _ := collect(t, root, files.WithFileTypes(files.FileTypeDir))
	want := []string{"empty", "sub"}
	if !slices.Equal(names, want) {
		t.Errorf("got %v, want %v", names, want)
	}
}

func TestWalk_SkipHidden(t *testing.T) {
	root := makeTree(t)
	names, _ := collect(t, root, files.WithSkipHidden())
	want := []string{"a.so", "b.SO", "empty", "notes.txt", "sub"}
	if !slices.Equal(names, want) {
		t.Errorf("got %v, want %v", names, want)
	}
}

func TestWalk_Recursive(t *testing.T) {
	root := makeTree(t)
	names, _ := collect(t, root, files.WithRecursive())
	want := []string{".hidden", "a.so", "b.SO", "empty", "notes.txt", "sub", "sub/c.so", "sub/d.yaml"}
	if !slices.Equal(names, want) {
		t.Errorf("got %v, want %v", names, want)
	}
}

func TestWalk_RecursiveExtensionFilter(t *testing.T) {
	root := makeTree(t)
	names, _ := collect(t, root,
		files.WithRecursive(),
		files.WithExtensions(".so"),
		files.WithFileTypes(files.FileTypeRegular),
	)
	want := []string{"a.so", "b.SO", "sub/c.so"}
	if !slices.Equal(names, want) {
		t.Errorf("got %v, want %v", names, want)
	}
}

func TestWalk_MaxDepth(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deep, "leaf.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "top.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Depth 0: only top-level.
	names, _ := collect(t, root, files.WithMaxDepth(0))
	want := []string{"a", "top.txt"}
	if !slices.Equal(names, want) {
		t.Errorf("depth=0: got %v, want %v", names, want)
	}

	// Depth 1: top + immediate children.
	names, _ = collect(t, root, files.WithMaxDepth(1))
	want = []string{"a", "a/b", "top.txt"}
	if !slices.Equal(names, want) {
		t.Errorf("depth=1: got %v, want %v", names, want)
	}

	// Unlimited via WithRecursive.
	names, _ = collect(t, root, files.WithRecursive())
	want = []string{"a", "a/b", "a/b/c", "a/b/c/leaf.txt", "top.txt"}
	if !slices.Equal(names, want) {
		t.Errorf("recursive: got %v, want %v", names, want)
	}
}

func TestWalk_DefaultMaxRecursionDepth(t *testing.T) {
	// The default cap must be a positive value high enough for realistic
	// trees. 256 is arbitrary but the test guards against accidental
	// regressions to 0 or a negative value that would change semantics.
	if files.DefaultMaxRecursionDepth < 64 {
		t.Errorf("DefaultMaxRecursionDepth = %d, want >= 64", files.DefaultMaxRecursionDepth)
	}
}

func TestWalk_UnboundedDepth(t *testing.T) {
	// Build a small tree well within both the cap and filesystem limits;
	// the assertion is that WithUnboundedDepth reaches the leaf.
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c", "d")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deep, "leaf.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	var found int
	for entry, err := range files.Walk(root,
		files.WithUnboundedDepth(),
		files.WithFileTypes(files.FileTypeRegular),
	) {
		if err != nil {
			t.Fatalf("Walk(WithUnboundedDepth): unexpected error: %v", err)
		}
		if entry.Name() == "leaf.txt" {
			found++
		}
	}
	if found != 1 {
		t.Errorf("Walk(WithUnboundedDepth): expected 1 leaf, got %d", found)
	}
}

func TestWalk_LastOptionWins(t *testing.T) {
	// Build a tree of depth 3 so that:
	//   - WithMaxDepth(1) sees only d0 (no leaf)
	//   - WithRecursive sees everything (cap is 256)
	//   - WithUnboundedDepth sees everything
	root := t.TempDir()
	d0 := filepath.Join(root, "d0")
	d1 := filepath.Join(d0, "d1")
	d2 := filepath.Join(d1, "d2")
	if err := os.MkdirAll(d2, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d2, "leaf.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		opts []files.WalkOption
		want int // expected leaf.txt count
	}{
		{
			name: "MaxDepth then Recursive — Recursive wins",
			opts: []files.WalkOption{files.WithMaxDepth(1), files.WithRecursive()},
			want: 1,
		},
		{
			name: "Recursive then MaxDepth(1) — MaxDepth wins, leaf unreachable",
			opts: []files.WalkOption{files.WithRecursive(), files.WithMaxDepth(1)},
			want: 0,
		},
		{
			name: "Unbounded then MaxDepth(1) — MaxDepth wins",
			opts: []files.WalkOption{files.WithUnboundedDepth(), files.WithMaxDepth(1)},
			want: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := append([]files.WalkOption{files.WithFileTypes(files.FileTypeRegular)}, tc.opts...)
			var got int
			for _, err := range files.Walk(root, opts...) {
				if err != nil {
					t.Fatalf("Walk: unexpected error: %v", err)
				}
				got++
			}
			if got != tc.want {
				t.Errorf("got %d leaf files, want %d", got, tc.want)
			}
		})
	}
}

func TestWalk_NonexistentRoot(t *testing.T) {
	const root = "/definitely/does/not/exist/xyz123"
	_, errs := collect(t, root)
	if len(errs) != 1 {
		t.Fatalf("Walk(%q): expected 1 error, got %d: %v", root, len(errs), errs)
	}
}

func TestWalk_EarlyStop(t *testing.T) {
	root := makeTree(t)
	count := 0
	for entry, err := range files.Walk(root, files.WithRecursive()) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		count++
		if count == 2 {
			// Break out of the loop early.
			_ = entry
			break
		}
	}
	if count != 2 {
		t.Errorf("expected to stop at 2, got %d", count)
	}
}

func TestWalk_EmptyDir(t *testing.T) {
	root := t.TempDir()
	names, errs := collect(t, root)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(names) != 0 {
		t.Errorf("expected no entries, got %v", names)
	}
}

func TestWalk_SymlinkNotFollowedByDefault(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks unreliable on windows in CI")
	}
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "deep.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}

	// Without follow: link is yielded but not descended.
	names, _ := collect(t, root, files.WithRecursive())
	want := []string{"link", "target", "target/deep.txt"}
	if !slices.Equal(names, want) {
		t.Errorf("no-follow: got %v, want %v", names, want)
	}

	// With follow: link is descended.
	names, _ = collect(t, root, files.WithRecursive(), files.WithFollowSymlinks())
	want = []string{"link", "link/deep.txt", "target", "target/deep.txt"}
	if !slices.Equal(names, want) {
		t.Errorf("follow: got %v, want %v", names, want)
	}
}

func TestWalk_RecursiveErrorContinues(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission semantics differ on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses permission errors")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(root, "noaccess")
	if err := os.Mkdir(bad, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(bad, 0o755) })
	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	var names []string
	var errCount int
	for entry, err := range files.Walk(root, files.WithRecursive()) {
		if err != nil {
			// Permission semantics vary across platforms; the contract under test
			// is that the iteration continues after an unreadable subdirectory.
			errCount++
			continue
		}
		names = append(names, entry.Name())
	}
	if errCount == 0 {
		t.Errorf("Walk(%q) recursive: expected at least one error from inaccessible dir, got none", root)
	}
	// Should still have visited a.txt, b.txt, and noaccess (the dir entry).
	slices.Sort(names)
	if len(names) < 3 {
		t.Errorf("Walk(%q) recursive: expected at least 3 entries despite error, got %v", root, names)
	}
}

