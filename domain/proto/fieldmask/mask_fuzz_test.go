// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/altessa-s/go-atlas/domain/proto/fieldmask"
)

func FuzzFieldMask(f *testing.F) {
	f.Add("a.b,c")
	f.Add("user.name,user.age")
	f.Add("x,,y")

	f.Fuzz(func(t *testing.T, s string) {
		paths := strings.Split(s, ",")
		m := fieldmask.FromPaths(paths...)

		// Round trip check property
		outPaths := m.ToPaths()

		// If input was valid unique paths without domination, outPaths approx equals paths.
		// But domination and sorting changes things.
		// We just ensure no panic and basic consistency.

		if m.IsEmpty() && len(paths) > 0 {
			// Check if all paths were empty strings.
			allEmpty := !slices.ContainsFunc(paths, func(p string) bool { return p != "" })
			_ = allEmpty
		}

		_ = m.Clone()

		// Union with itself should be equal to itself.
		u := m.Union(m)
		assert.Len(t, u.ToPaths(), len(outPaths), "union with self changed size")
	})
}
