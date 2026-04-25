// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package hash

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func FuzzSHA256Determinism(f *testing.F) {
	f.Add("hello")
	f.Add("")
	f.Add("test input with special chars: !@#$%")

	f.Fuzz(func(t *testing.T, input string) {
		h1 := SHA256HexString(input)
		h2 := SHA256HexString(input)
		require.Equal(t, h1, h2, "non-deterministic: SHA256HexString(%q)", input)
	})
}
