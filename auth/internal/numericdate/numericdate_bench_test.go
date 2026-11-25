// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package numericdate_test

import (
	"encoding/json"
	"testing"

	"github.com/altessa-s/go-atlas/auth/internal/numericdate"
)

func BenchmarkParse(b *testing.B) {
	for _, n := range []json.Number{"1750000000", "1750000000.000000001", "1.7500000000000000001e9"} {
		b.Run(n.String(), func(b *testing.B) {
			for b.Loop() {
				numericdate.Parse(n)
			}
		})
	}
}
