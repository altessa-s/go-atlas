// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lua

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func BenchmarkTranslate_Simple(b *testing.B) {
	node := testhelpers.MustParseFilter(b, `name == "John"`)
	trans := NewTranslator("")

	for b.Loop() {
		_, _ = trans.Translate(node)
	}
}

func BenchmarkTranslate_Complex(b *testing.B) {
	node := testhelpers.MustParseFilter(b,
		`(name == "John" || name == "Jane") && age >= 18 && status in ["active", "pending"]`)
	trans := NewTranslator("")

	for b.Loop() {
		_, _ = trans.Translate(node)
	}
}

func BenchmarkTranslate_FieldMapping(b *testing.B) {
	node := testhelpers.MustParseFilter(b,
		`organizationIds in ["org-1", "org-2"] && status == 2`)
	trans := NewTranslator("row", filter.WithFieldMapping(map[string]string{
		"organizationIds": "organization_id",
	}))

	for b.Loop() {
		_, _ = trans.Translate(node)
	}
}
