// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package orderby_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/orderby"
)

const benchExpr = "create_time desc, organization_id asc, slug, address.city desc, updated_at"

func BenchmarkParse_Cached(b *testing.B) {
	p, err := orderby.NewParser()
	if err != nil {
		b.Fatal(err)
	}
	ctx := b.Context()

	for b.Loop() {
		_, _ = p.Parse(ctx, benchExpr)
	}
}

func BenchmarkParse_NoCache(b *testing.B) {
	p, err := orderby.NewParser(orderby.WithParserNoCache())
	if err != nil {
		b.Fatal(err)
	}
	ctx := b.Context()

	for b.Loop() {
		_, _ = p.Parse(ctx, benchExpr)
	}
}

func BenchmarkParse_SingleKey(b *testing.B) {
	p, err := orderby.NewParser(orderby.WithParserNoCache())
	if err != nil {
		b.Fatal(err)
	}
	ctx := b.Context()

	for b.Loop() {
		_, _ = p.Parse(ctx, "create_time desc")
	}
}
