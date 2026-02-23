// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fallback

import "testing"

func FuzzBehavior_IsValid(f *testing.F) {
	f.Add("allow")
	f.Add("deny")
	f.Add("error")
	f.Add("")
	f.Add("unknown")

	f.Fuzz(func(t *testing.T, s string) {
		b := Behavior(s)
		valid := b.IsValid()
		if valid && !(b == Allow || b == Deny || b == Error) {
			t.Fatalf("IsValid() returned true for %q", s)
		}
	})
}
