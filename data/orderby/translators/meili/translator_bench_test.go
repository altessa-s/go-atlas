// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package meili_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func BenchmarkTranslate_Simple(b *testing.B) {
	ob := testhelpers.MustParseOrderBy(b, "createdAt desc")
	trans := mustTranslator(b)

	for b.Loop() {
		_, _ = trans.Translate(ob)
	}
}

func BenchmarkTranslate_Multi(b *testing.B) {
	ob := testhelpers.MustParseOrderBy(b,
		"createdAt desc, organization_id asc, slug, address.city desc, updated_at")
	trans := mustTranslator(b)

	for b.Loop() {
		_, _ = trans.Translate(ob)
	}
}
