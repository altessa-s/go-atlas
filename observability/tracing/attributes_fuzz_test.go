// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func FuzzStringAttribute(f *testing.F) {
	f.Add("key", "value")
	f.Add("", "value")
	f.Add("key", "")
	f.Add("http.method", "GET")

	f.Fuzz(func(t *testing.T, key, value string) {
		attr := String(key, value)
		assert.Equal(t, key, attr.Key)
		assert.Equal(t, value, attr.Value)
		if key == "" {
			assert.False(t, attr.Valid(), "empty key should be invalid")
		} else {
			assert.True(t, attr.Valid(), "non-empty key should be valid")
		}
	})
}

func FuzzFilterAttributesByKey(f *testing.F) {
	f.Add("http.")
	f.Add("")
	f.Add("db.system")

	f.Fuzz(func(t *testing.T, prefix string) {
		attrs := []Attribute{
			String("http.method", "GET"),
			String("db.system", "pg"),
			String("rpc.method", "call"),
		}
		count := 0
		for range FilterAttributesByKey(attrs, prefix) {
			count++
		}
		assert.LessOrEqual(t, count, len(attrs), "filtered more than total")
	})
}
