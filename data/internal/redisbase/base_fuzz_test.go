// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisbase_test

import (
	"testing"
)

func FuzzBase_Key(f *testing.F) {
	f.Add("prefix", "key")
	f.Add("", "key")
	f.Add("prefix", "")
	f.Add("a:b:c", "d:e:f")

	base, _ := setupBase(f, "testprefix")
	ctx := f.Context()

	f.Fuzz(func(t *testing.T, prefix, key string) {
		// Should not panic
		_ = base.Key(key)
		_, _ = base.Exists(ctx, key)
	})
}
