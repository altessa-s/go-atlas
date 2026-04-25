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

	"github.com/stretchr/testify/require"

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
		require.NoError(t, relErr, "rel(%q, %q)", root, entry.Path)
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
		require.NoError(t, os.WriteFile(filepath.Join(root, p), []byte("x"), 0o644))
	}
	mustMkdir := func(p string) {
		require.NoError(t, os.MkdirAll(filepath.Join(root, p), 0o755))
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
	require.Empty(t, errs, "unexpected errors")
	want := []string{".hidden", "a.so", "b.SO", "empty", "notes.txt", "sub"}
	require.Equal(t, want, names)
}

func TestWalk_ExtensionFilter_CaseInsensitive(t *testing.T) {
	root := makeTree(t)
	names, errs := collect(t, root, files.WithExtensions(".so"))
	require.Empty(t, errs, "unexpected errors")
	// Both a.so and b.SO should match (case-insensitive).
	want := []string{"a.so", "b.SO"}
	require.Equal(t, want, names)
}

func TestWalk_ExtensionFilter_NormalizesLeadingDot(t *testing.T) {
	root := makeTree(t)
	names, _ := collect(t, root, files.WithExtensions("so", "TXT"))
	want := []string{"a.so", "b.SO", "notes.txt"}
	require.Equal(t, want, names)
}

func TestWalk_FileTypes_RegularOnly(t *testing.T) {
	root := makeTree(t)
	names, _ := collect(t, root, files.WithFileTypes(files.FileTypeRegular))
	// Excludes "sub" and "empty" directories.
	want := []string{".hidden", "a.so", "b.SO", "notes.txt"}
	require.Equal(t, want, names)
}

func TestWalk_FileTypes_DirOnly(t *testing.T) {
	root := makeTree(t)
	names, _ := collect(t, root, files.WithFileTypes(files.FileTypeDir))
	want := []string{"empty", "sub"}
	require.Equal(t, want, names)
}

func TestWalk_SkipHidden(t *testing.T) {
	root := makeTree(t)
	names, _ := collect(t, root, files.WithSkipHidden())
	want := []string{"a.so", "b.SO", "empty", "notes.txt", "sub"}
	require.Equal(t, want, names)
}

func TestWalk_Recursive(t *testing.T) {
	root := makeTree(t)
	names, _ := collect(t, root, files.WithRecursive())
	want := []string{".hidden", "a.so", "b.SO", "empty", "notes.txt", "sub", "sub/c.so", "sub/d.yaml"}
	require.Equal(t, want, names)
}

func TestWalk_RecursiveExtensionFilter(t *testing.T) {
	root := makeTree(t)
	names, _ := collect(t, root,
		files.WithRecursive(),
		files.WithExtensions(".so"),
		files.WithFileTypes(files.FileTypeRegular),
	)
	want := []string{"a.so", "b.SO", "sub/c.so"}
	require.Equal(t, want, names)
}

func TestWalk_MaxDepth(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c")
	require.NoError(t, os.MkdirAll(deep, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(deep, "leaf.txt"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "top.txt"), []byte("x"), 0o644))

	// Depth 0: only top-level.
	names, _ := collect(t, root, files.WithMaxDepth(0))
	want := []string{"a", "top.txt"}
	require.Equal(t, want, names, "depth=0")

	// Depth 1: top + immediate children.
	names, _ = collect(t, root, files.WithMaxDepth(1))
	want = []string{"a", "a/b", "top.txt"}
	require.Equal(t, want, names, "depth=1")

	// Unlimited via WithRecursive.
	names, _ = collect(t, root, files.WithRecursive())
	want = []string{"a", "a/b", "a/b/c", "a/b/c/leaf.txt", "top.txt"}
	require.Equal(t, want, names, "recursive")
}

func TestWalk_DefaultMaxRecursionDepth(t *testing.T) {
	// The default cap must be a positive value high enough for realistic
	// trees. 256 is arbitrary but the test guards against accidental
	// regressions to 0 or a negative value that would change semantics.
	require.GreaterOrEqual(t, files.DefaultMaxRecursionDepth, 64, "DefaultMaxRecursionDepth")
}

func TestWalk_UnboundedDepth(t *testing.T) {
	// Build a small tree well within both the cap and filesystem limits;
	// the assertion is that WithUnboundedDepth reaches the leaf.
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c", "d")
	require.NoError(t, os.MkdirAll(deep, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(deep, "leaf.txt"), []byte("x"), 0o644))

	var found int
	for entry, err := range files.Walk(root,
		files.WithUnboundedDepth(),
		files.WithFileTypes(files.FileTypeRegular),
	) {
		require.NoError(t, err, "Walk(WithUnboundedDepth)")
		if entry.Name() == "leaf.txt" {
			found++
		}
	}
	require.Equal(t, 1, found, "Walk(WithUnboundedDepth): expected 1 leaf")
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
	require.NoError(t, os.MkdirAll(d2, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(d2, "leaf.txt"), []byte("x"), 0o644))

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
				require.NoError(t, err, "Walk")
				got++
			}
			require.Equal(t, tc.want, got, "leaf file count")
		})
	}
}

func TestWalk_NonexistentRoot(t *testing.T) {
	const root = "/definitely/does/not/exist/xyz123"
	_, errs := collect(t, root)
	require.Len(t, errs, 1, "Walk(%q): expected 1 error", root)
}

func TestWalk_EarlyStop(t *testing.T) {
	root := makeTree(t)
	count := 0
	for entry, err := range files.Walk(root, files.WithRecursive()) {
		require.NoError(t, err)
		count++
		if count == 2 {
			// Break out of the loop early.
			_ = entry
			break
		}
	}
	require.Equal(t, 2, count, "expected to stop at 2")
}

func TestWalk_EmptyDir(t *testing.T) {
	root := t.TempDir()
	names, errs := collect(t, root)
	require.Empty(t, errs, "unexpected errors")
	require.Empty(t, names, "expected no entries")
}

func TestWalk_SymlinkNotFollowedByDefault(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks unreliable on windows in CI")
	}
	root := t.TempDir()
	target := filepath.Join(root, "target")
	require.NoError(t, os.Mkdir(target, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(target, "deep.txt"), []byte("x"), 0o644))
	require.NoError(t, os.Symlink(target, filepath.Join(root, "link")))

	// Without follow: link is yielded but not descended.
	names, _ := collect(t, root, files.WithRecursive())
	want := []string{"link", "target", "target/deep.txt"}
	require.Equal(t, want, names, "no-follow")

	// With follow: link is descended.
	names, _ = collect(t, root, files.WithRecursive(), files.WithFollowSymlinks())
	want = []string{"link", "link/deep.txt", "target", "target/deep.txt"}
	require.Equal(t, want, names, "follow")
}

func TestWalk_BrokenSymlinkSurfacesError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks unreliable on windows in CI")
	}
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "good.txt"), []byte("x"), 0o644))
	// Symlink pointing at a non-existent target. With WithFollowSymlinks
	// the walker tries to stat the target and must surface the failure
	// instead of silently treating the link as a non-directory entry.
	require.NoError(t, os.Symlink(filepath.Join(root, "does-not-exist"), filepath.Join(root, "broken")))

	_, errs := collect(t, root, files.WithRecursive(), files.WithFollowSymlinks())
	require.Len(t, errs, 1, "expected exactly 1 error from broken symlink")
	require.Contains(t, errs[0].Error(), "broken", "error should reference symlink path 'broken'")
	require.Contains(t, errs[0].Error(), "stat symlink target", "error should reference 'stat symlink target' operation")
}

func TestWalk_RecursiveErrorContinues(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission semantics differ on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses permission errors")
	}
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte("x"), 0o644))
	bad := filepath.Join(root, "noaccess")
	require.NoError(t, os.Mkdir(bad, 0o000))
	t.Cleanup(func() { _ = os.Chmod(bad, 0o755) })
	require.NoError(t, os.WriteFile(filepath.Join(root, "b.txt"), []byte("x"), 0o644))

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
	require.NotZero(t, errCount, "Walk(%q) recursive: expected at least one error from inaccessible dir", root)
	// Should still have visited a.txt, b.txt, and noaccess (the dir entry).
	slices.Sort(names)
	require.GreaterOrEqual(t, len(names), 3, "Walk(%q) recursive: expected at least 3 entries despite error, got %v", root, names)
}
