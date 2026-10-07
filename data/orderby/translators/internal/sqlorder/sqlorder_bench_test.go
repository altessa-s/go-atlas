// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqlorder_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/orderby/translators/internal/sqlorder"
	"github.com/altessa-s/go-atlas/internal/sqldialect"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func BenchmarkTranslate(b *testing.B) {
	cfg := context(b)
	ob := testhelpers.MustParseOrderBy(b, "create_time desc, organization_id, slug, address.city desc, updated_at")
	b.ReportAllocs()
	for b.Loop() {
		if _, err := sqlorder.Translate(cfg, sqldialect.Postgres, ob); err != nil {
			b.Fatal(err)
		}
	}
}
