// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func FuzzValidateLabelNames(f *testing.F) {
	f.Add("method")
	f.Add("")
	f.Add("status_code")
	f.Add("http.method")

	f.Fuzz(func(t *testing.T, name string) {
		err := ValidateLabelNames([]string{name})
		if name == "" {
			assert.Error(t, err, "expected error for empty label name")
		} else {
			assert.NoError(t, err, "unexpected error for %q", name)
		}
	})
}

func FuzzNormalizeLabelNames(f *testing.F) {
	f.Add("b,a,c")

	f.Fuzz(func(t *testing.T, input string) {
		names := []string{input}
		result := NormalizeLabelNames(names)
		assert.Len(t, result, 1)
	})
}
