// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"testing"
)

func FuzzStorage_AttemptLock(f *testing.F) {
	f.Add("key1", []byte("value1"))
	f.Add("", []byte(""))
	f.Add("special:key", []byte("data"))

	f.Fuzz(func(t *testing.T, key string, val []byte) {
		s := New()
		ctx := t.Context()
		_, _, lockToken, _ := s.AttemptLock(ctx, key, val)
		_ = s.Complete(ctx, key, val, lockToken)
		_ = s.Delete(ctx, key)
	})
}
