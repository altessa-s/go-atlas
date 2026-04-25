// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func FuzzIsValidUUIDv4(f *testing.F) {
	f.Add("550e8400-e29b-41d4-a716-446655440000")
	f.Add("")
	f.Add("not-a-uuid")
	f.Add("550e8400-e29b-41d4-c716-446655440000") // wrong variant

	f.Fuzz(func(t *testing.T, s string) {
		result := IsValidUUIDv4(s)
		if result {
			assert.Len(t, s, UUIDLength)
			assert.Equal(t, byte('4'), s[14])
		}
	})
}
