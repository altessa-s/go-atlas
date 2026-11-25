// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package requestid

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func FuzzGenerator_Extract(f *testing.F) {
	f.Add("550e8400-e29b-41d4-a716-446655440000")
	f.Add("")
	f.Add("invalid-uuid")
	f.Add("not-even-close")

	f.Fuzz(func(t *testing.T, headerValue string) {
		gen := NewGenerator(WithUUIDGenerator(func() string { return "fallback" }))
		headers := &fuzzHeaderGetter{value: headerValue}
		result := gen.Extract(headers)
		assert.NotEqual(t, "", result)
	})
}

type fuzzHeaderGetter struct{ value string }

func (f *fuzzHeaderGetter) GetHeader(_ string) string { return f.value }
