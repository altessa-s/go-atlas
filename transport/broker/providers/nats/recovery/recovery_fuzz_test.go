// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import "testing"

func FuzzRecoveryStrategy_String(f *testing.F) {
	f.Add(0)
	f.Add(1)
	f.Add(2)
	f.Add(99)

	f.Fuzz(func(t *testing.T, val int) {
		s := RecoveryStrategy(val)
		result := s.String()
		if result == "" {
			t.Fatal("String() returned empty")
		}
	})
}
