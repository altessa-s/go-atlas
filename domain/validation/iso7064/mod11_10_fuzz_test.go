// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package iso7064_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

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

		if valid {
			assert.NoError(t, err, "Mod11_10(%q) returned valid=true but err=%v", in, err)
		}

		isValidFunc := iso7064.IsValidMod11_10(in)
		if err != nil {
			assert.False(t, isValidFunc, "IsValid returned true for errored input: %s", in)
		} else {
			assert.Equal(t, valid, isValidFunc, "Consistency mismatch: Mod11_10 said %v, IsValid said %v", valid, isValidFunc)
		}
	})
}
