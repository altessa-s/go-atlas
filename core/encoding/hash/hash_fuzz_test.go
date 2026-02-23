// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package hash

import "testing"

func FuzzSHA256Determinism(f *testing.F) {
	f.Add("hello")
	f.Add("")
	f.Add("test input with special chars: !@#$%")

	f.Fuzz(func(t *testing.T, input string) {
		h1 := SHA256HexString(input)
		h2 := SHA256HexString(input)
		if h1 != h2 {
			t.Errorf("non-deterministic: SHA256HexString(%q) returned %q then %q", input, h1, h2)
		}
	})
}
