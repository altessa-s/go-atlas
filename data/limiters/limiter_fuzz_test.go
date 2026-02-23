// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiters

import "testing"

func FuzzLimitInfo_IsLimitExceeded(f *testing.F) {
	f.Add(int64(0))
	f.Add(int64(1))
	f.Add(int64(-1))
	f.Add(int64(100))

	f.Fuzz(func(t *testing.T, remaining int64) {
		li := &LimitInfo{Remaining: remaining}
		got := li.IsLimitExceeded()
		want := remaining <= 0
		if got != want {
			t.Fatalf("IsLimitExceeded() with remaining=%d: got %v, want %v", remaining, got, want)
		}
	})
}
