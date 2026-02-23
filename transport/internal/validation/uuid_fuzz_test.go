// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package validation

import "testing"

func FuzzIsValidUUIDv4(f *testing.F) {
	f.Add("550e8400-e29b-41d4-a716-446655440000")
	f.Add("")
	f.Add("not-a-uuid")
	f.Add("550e8400-e29b-41d4-c716-446655440000") // wrong variant

	f.Fuzz(func(t *testing.T, s string) {
		result := IsValidUUIDv4(s)
		if result {
			if len(s) != UUIDLength {
				t.Fatalf("accepted string of length %d", len(s))
			}
			if s[14] != '4' {
				t.Fatal("accepted non-v4 UUID")
			}
		}
	})
}
