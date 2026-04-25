// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import "testing"

func FuzzDefaultKeyValidator(f *testing.F) {
	f.Add("550e8400-e29b-41d4-a716-446655440000")
	f.Add("")
	f.Add("not-a-uuid")
	f.Add("550E8400-E29B-41D4-A716-446655440000")
	f.Fuzz(func(t *testing.T, key string) {
		DefaultKeyValidator(key) //nolint:errcheck
	})
}
