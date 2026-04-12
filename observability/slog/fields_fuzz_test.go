// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package slog

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func FuzzFieldsToAttrs(f *testing.F) {
	f.Add("key", "value")
	f.Add("http.method", "GET")
	f.Add("a.b.c", "nested")
	f.Add("", "empty-key")

	f.Fuzz(func(t *testing.T, key, value string) {
		fields := Fields{{Key: key, Value: value}}
		// Should not panic
		result := FieldsToAttrs(fields)
		if key != "" {
			assert.NotEmpty(t, result, "expected at least one attr for non-empty key")
		}
	})
}

func FuzzFields_Delete(f *testing.F) {
	f.Add("a")
	f.Add("")
	f.Add("missing")

	f.Fuzz(func(t *testing.T, key string) {
		fields := Fields{
			{Key: "a", Value: 1},
			{Key: "b", Value: 2},
		}
		result := fields.Delete(key)
		if key == "a" {
			assert.Len(t, result, 1)
		}
	})
}
