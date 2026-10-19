// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package googletype_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/domain/money"
	"github.com/altessa-s/go-atlas/domain/proto/googletype"

	"google.golang.org/genproto/googleapis/type/date"
	"google.golang.org/genproto/googleapis/type/dayofweek"

	gtypemoney "google.golang.org/genproto/googleapis/type/money"
)

func BenchmarkDateFromProto(b *testing.B) {
	d := &date.Date{Year: 2026, Month: 10, Day: 18}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := googletype.DateFromProto(d); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWeekdaysFromProto(b *testing.B) {
	days := []dayofweek.DayOfWeek{dayofweek.DayOfWeek_MONDAY, dayofweek.DayOfWeek_WEDNESDAY, dayofweek.DayOfWeek_FRIDAY}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := googletype.WeekdaysFromProto(days); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMoneyFromProto(b *testing.B) {
	p := &gtypemoney.Money{CurrencyCode: "RUB", Units: 1234, Nanos: 560_000_000}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := googletype.MoneyFromProto(p); err != nil {
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
		if _, err := googletype.MoneyToProto(m); err != nil {
			b.Fatal(err)
		}
	}
}
