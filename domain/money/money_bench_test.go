// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package money_test

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/altessa-s/go-atlas/domain/money"
)

func BenchmarkAllocate(b *testing.B) {
	m, err := money.FromMinor(1_000_000, eur)
	if err != nil {
		b.Fatal(err)
	}
	ratios := []decimal.Decimal{decimal.NewFromInt(3), decimal.NewFromInt(5), decimal.NewFromInt(7), decimal.NewFromInt(11)}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := m.Allocate(ratios...); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRound(b *testing.B) {
	m, err := money.Parse("1234.56789", "EUR")
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		_ = m.Round(money.HalfEven)
	}
}
