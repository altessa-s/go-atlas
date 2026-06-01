// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisearch

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func BenchmarkTranslate_Simple(b *testing.B) {
	node := testhelpers.MustParseFilter(b, `status == 1`)
	trans := mustTranslator(b, testSchema)

	for b.Loop() {
		_, _ = trans.Translate(node)
	}
}

func BenchmarkTranslate_Complex(b *testing.B) {
	node := testhelpers.MustParseFilter(b,
		`(status == 1 && priority >= 3) && (failures == 0 && age > 10)`)
	trans := mustTranslator(b, testSchema)

	for b.Loop() {
		_, _ = trans.Translate(node)
	}
}

func BenchmarkTranslate_FieldMapping(b *testing.B) {
	node := testhelpers.MustParseFilter(b, `mapped_status == 1`)
	trans := mustTranslator(b,
		map[string]FieldType{"mapped_status": FieldTypeNumeric},
		filter.WithFieldMapping(map[string]string{"status": "mapped_status"}),
	)

	for b.Loop() {
		_, _ = trans.Translate(node)
	}
}
