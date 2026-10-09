// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func BenchmarkCheckLimits(b *testing.B) {
	ctx, err := filter.NewTranslatorContext()
	require.NoError(b, err)
	node := testhelpers.MustParseFilter(b, `active && age >= 18 && status in ["active", "pending"]`)
	b.ReportAllocs()
	for b.Loop() {
		if err := ctx.CheckLimits(node); err != nil {
			b.Fatal(err)
		}
	}
}
