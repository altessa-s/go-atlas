// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import "testing"

func FuzzValidateLabelNames(f *testing.F) {
	f.Add("method")
	f.Add("")
	f.Add("status_code")
	f.Add("http.method")

	f.Fuzz(func(t *testing.T, name string) {
		err := ValidateLabelNames([]string{name})
		if name == "" && err == nil {
			t.Error("expected error for empty label name")
		}
		if name != "" && err != nil {
			t.Errorf("unexpected error for %q: %v", name, err)
		}
	})
}

func FuzzNormalizeLabelNames(f *testing.F) {
	f.Add("b,a,c")

	f.Fuzz(func(t *testing.T, input string) {
		names := []string{input}
		result := NormalizeLabelNames(names)
		if len(result) != 1 {
			t.Errorf("expected 1 element, got %d", len(result))
		}
	})
}
