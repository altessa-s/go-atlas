// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func BenchmarkTranslate_Simple(b *testing.B) {
	node := testhelpers.MustParseFilter(b, `name == "John"`)
	trans := mustTranslator(b)

	for b.Loop() {
		_, _ = trans.Translate(node)
	}
}

func BenchmarkTranslate_Complex(b *testing.B) {
	node := testhelpers.MustParseFilter(b,
		`(name == "John" || name == "Jane") && age >= 18 && status in ["active", "pending"]`)
	trans := mustTranslator(b)

	for b.Loop() {
		_, _ = trans.Translate(node)
	}
}

func BenchmarkTranslate_FieldMapping(b *testing.B) {
	node := testhelpers.MustParseFilter(b,
		`organizationIds in ["org-1", "org-2"] && status == 2`)
	trans := mustTranslator(b, filter.WithFieldMapping(map[string]string{
		"organizationIds": "organization_id",
	}))

	for b.Loop() {
		_, _ = trans.Translate(node)
	}
}

func BenchmarkTranslate_ZeroWhenAbsent(b *testing.B) {
	node := testhelpers.MustParseFilter(b,
		`description == "" && !oneShot && nextRunAt < 100 && description.matches("^a")`)
	trans := mustTranslator(b, filter.WithZeroWhenAbsent(map[string]filter.FieldKind{
		"description": filter.FieldKindString,
		"oneShot":     filter.FieldKindBool,
		"nextRunAt":   filter.FieldKindInt,
	}))

	for b.Loop() {
		_, _ = trans.Translate(node)
	}
}
