// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package slog

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

func FuzzMaskingReplaceAttr(f *testing.F) {
	f.Add("password", "secret-value", "***")
	f.Add("", "value", "MASKED")
	f.Add("token", "", "***")

	f.Fuzz(func(t *testing.T, key, value, mask string) {
		fn := MaskingReplaceAttr([]string{key}, mask)
		attr := slog.String(key, value)
		result := fn(nil, attr)
		if key != "" {
			assert.Equal(t, mask, result.Value.String())
		}
	})
}

func FuzzString(f *testing.F) {
	f.Add("key", "value")
	f.Add("", "value")
	f.Add("key", "")

	f.Fuzz(func(t *testing.T, key, value string) {
		attr := String(key, value)
		if value == "" {
			assert.Empty(t, attr.Key, "empty value should produce empty attr")
		}
	})
}
