// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter_test

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func BenchmarkEvaluateDecimalBounds(b *testing.B) {
	eval, err := filter.NewEvaluator(filter.WithFieldTypes(map[string]filter.FieldKind{"price": filter.FieldKindDecimal}))
	if err != nil {
		b.Fatal(err)
	}
	node := testhelpers.MustParseFilter(b, `price >= 100000.5 && price <= 5000000`)
	data := map[string]any{"price": decimal.RequireFromString("1500000.50")}
	for b.Loop() {
		if _, err := eval.Evaluate(node, data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecimalLiteral(b *testing.B) {
	b.Run("float", func(b *testing.B) {
		for b.Loop() {
			_, _ = filter.DecimalLiteral(1000000.5)
		}
	})
	b.Run("string", func(b *testing.B) {
		for b.Loop() {
			_, _ = filter.DecimalLiteral("12345678901234567.89")
		}
	})
}
