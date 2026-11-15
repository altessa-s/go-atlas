// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package moneypb_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/domain/money"
	"github.com/altessa-s/go-atlas/domain/money/moneypb"

	gtypemoney "google.golang.org/genproto/googleapis/type/money"
)

func BenchmarkMoneyFromProto(b *testing.B) {
	p := &gtypemoney.Money{CurrencyCode: "RUB", Units: 1234, Nanos: 560_000_000}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := moneypb.MoneyFromProto(p); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMoneyToProto(b *testing.B) {
	m, err := money.Parse("1234.56", "RUB")
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := moneypb.MoneyToProto(m); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMoneyToProtoFitted(b *testing.B) {
	m, err := money.Parse("1234.5678901234", "RUB")
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if p := moneypb.MoneyToProtoFitted(m); p == nil {
			b.Fatal("nil")
		}
	}
}
