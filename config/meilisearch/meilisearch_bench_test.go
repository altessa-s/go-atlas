// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package meilisearchconfig_test

import (
	"testing"

	meilisearchconfig "github.com/altessa-s/go-atlas/config/meilisearch"
)

func BenchmarkConfigValidate(b *testing.B) {
	cfg := meilisearchconfig.Default()
	b.ReportAllocs()
	for b.Loop() {
		if err := cfg.Validate(); err != nil {
			b.Fatal(err)
		}
	}
}
