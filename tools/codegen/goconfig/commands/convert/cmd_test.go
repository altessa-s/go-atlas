// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package convert

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestIsPathWithinDir(t *testing.T) {
	base := filepath.Join(string(filepath.Separator), "tmp", "goconfig-test")

	t.Run("inside", func(t *testing.T) {
		p := filepath.Join(base, "a", "b.yaml")
		if !isPathWithinDir(base, p) {
			t.Fatalf("expected path to be within dir: dir=%q path=%q", base, p)
		}
	})

	t.Run("escape", func(t *testing.T) {
		p := filepath.Join(base, "..", "other", "x.yaml")
		if isPathWithinDir(base, p) {
			t.Fatalf("expected path to be outside dir: dir=%q path=%q", base, p)
		}
	})

	t.Run("prefix-trick", func(t *testing.T) {
		// Historically, HasPrefix-based checks can be fooled by /tmp/a vs /tmp/ab.
		// Rel-based checks should treat /tmp/ab/* as outside /tmp/a.
		if runtime.GOOS == "windows" {
			t.Skip("path semantics differ on Windows; covered by other cases")
		}
		dir := filepath.Join(string(filepath.Separator), "tmp", "a")
		p := filepath.Join(string(filepath.Separator), "tmp", "ab", "x.yaml")
		if isPathWithinDir(dir, p) {
			t.Fatalf("expected outside: dir=%q path=%q", dir, p)
		}
	})
}
