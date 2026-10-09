// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package civil_test

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/core/time/civil"
)

func BenchmarkParse(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := civil.Parse("2026-10-09"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkString(b *testing.B) {
	d := civil.MustNew(2026, time.October, 9)
	b.ReportAllocs()
	for b.Loop() {
		_ = d.String()
	}
}

func BenchmarkAddMonthsClamped(b *testing.B) {
	d := civil.MustNew(2026, time.January, 31)
	b.ReportAllocs()
	for b.Loop() {
		_ = d.AddMonthsClamped(13, 31)
	}
}

func BenchmarkDaysSince(b *testing.B) {
	d, o := civil.MustNew(2026, time.October, 9), civil.MustNew(2000, time.January, 1)
	b.ReportAllocs()
	for b.Loop() {
		_ = d.DaysSince(o)
	}
}
