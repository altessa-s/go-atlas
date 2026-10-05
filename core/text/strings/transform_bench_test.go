// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package strings_test

import (
	"testing"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

func BenchmarkToCamelCase(b *testing.B) {
	b.Run("ASCII", func(b *testing.B) {
		for b.Loop() {
			corestrings.ToCamelCase("max_retry_count_per_request")
		}
	})
	b.Run("Unicode", func(b *testing.B) {
		for b.Loop() {
			corestrings.ToCamelCase("größe_über_alles_müller")
		}
	})
}

func BenchmarkScreamingSnakeToCamelCase(b *testing.B) {
	b.Run("ASCII", func(b *testing.B) {
		for b.Loop() {
			corestrings.ScreamingSnakeToCamelCase("MAX_RETRY_COUNT_PER_REQUEST")
		}
	})
	b.Run("Unicode", func(b *testing.B) {
		for b.Loop() {
			corestrings.ScreamingSnakeToCamelCase("GRÖSSE_ÜBER_ALLES_MÜLLER")
		}
	})
}
