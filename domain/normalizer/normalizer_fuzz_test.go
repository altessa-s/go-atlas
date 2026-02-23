// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package normalizer_test

import (
	"strings"
	"testing"

	"github.com/altessa-s/go-atlas/domain/normalizer"
)

func FuzzNormalize(f *testing.F) {
	f.Add(" test ", "TEST")
	f.Add("", "")
	f.Add("   ", "a")

	f.Fuzz(func(t *testing.T, s1, s2 string) {
		type Data struct {
			TrimLower string `normalize:"trim,lowercase"`
			Upper     string `normalize:"uppercase"`
		}

		d := &Data{
			TrimLower: s1,
			Upper:     s2,
		}

		err := normalizer.Normalize(d)
		if err != nil {
			t.Errorf("Normalize failed: %v", err)
		}

		// Property checks
		if d.TrimLower != strings.ToLower(strings.TrimSpace(s1)) {
			t.Errorf("TrimLower mismatch for input %q: got %q", s1, d.TrimLower)
		}
		if d.Upper != strings.ToUpper(s2) {
			t.Errorf("Upper mismatch for input %q: got %q", s2, d.Upper)
		}
	})
}
