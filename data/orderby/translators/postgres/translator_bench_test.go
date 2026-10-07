// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func BenchmarkTranslate_Multi(b *testing.B) {
	ob := testhelpers.MustParseOrderBy(b, "create_time desc, organization_id asc, slug, address.city desc, updated_at")
	trans := mustTranslator(b)
	b.ReportAllocs()
	for b.Loop() {
		_, _ = trans.Translate(ob)
	}
}
