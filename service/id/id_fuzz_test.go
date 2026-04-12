// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package id

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func FuzzStatic_ID(f *testing.F) {
	f.Add("test-id")
	f.Add("")
	f.Add("550e8400-e29b-41d4-a716-446655440000")
	f.Add("very-long-id-" + string(make([]byte, 1000)))

	f.Fuzz(func(t *testing.T, idVal string) {
		p := NewStatic(idVal)
		assert.Equal(t, idVal, p.ID())
	})
}

func FuzzNewFile(f *testing.F) {
	f.Add("")
	f.Add("/tmp/test.id")
	f.Add("relative/path.id")

	f.Fuzz(func(t *testing.T, path string) {
		// Should not panic
		_, _ = NewFile(path)
	})
}
