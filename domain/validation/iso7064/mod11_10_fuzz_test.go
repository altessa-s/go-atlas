// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package iso7064_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/domain/validation/iso7064"
)

func FuzzMod11_10_String(f *testing.F) {
	f.Add("123456788")
	f.Add("000000001")
	f.Add("invalid")

	f.Fuzz(func(t *testing.T, in string) {
		valid, err := iso7064.Mod11_10(in)

		// Properties:
		// 1. If err is nil, valid should be consistent (either valid or not, but no panic)
		// 2. If valid is true, err MUST be nil.

		if valid && err != nil {
			t.Errorf("Mod11_10(%q) returned valid=true but err=%v", in, err)
		}

		isValidFunc := iso7064.IsValidMod11_10(in)
		if isValidFunc != valid {
			// IsValidMod11_10 returns false if err != nil OR !ok
			// So if err != nil, isValidFunc=false. valid=false. Match.
			// If err == nil:
			//   isValidFunc should equal valid.

			if err != nil {
				if isValidFunc {
					t.Errorf("IsValid returned true for errored input: %s", in)
				}
			} else {
				if isValidFunc != valid {
					t.Errorf("Consistency mismatch: Mod11_10 said %v, IsValid said %v", valid, isValidFunc)
				}
			}
		}
	})
}
