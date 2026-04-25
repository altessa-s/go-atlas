// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package converter_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/altessa-s/go-atlas/domain/converter"
)

type FuzzStruct struct {
	S string
	I int
	B bool
}

func FuzzConvert(f *testing.F) {
	f.Add("test", 123, true)
	f.Add("", 0, false)
	f.Add("long string", -1, true)

	f.Fuzz(func(t *testing.T, s string, i int, b bool) {
		src := FuzzStruct{S: s, I: i, B: b}
		var dst FuzzStruct

		// Should never panic
		converter.Convert(src, &dst)

		assert.Equal(t, s, dst.S)
		assert.Equal(t, i, dst.I)
		assert.Equal(t, b, dst.B)

	})
}
