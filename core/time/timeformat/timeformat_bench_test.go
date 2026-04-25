// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package timeformat

import (
	"testing"
	"time"
)

var benchInstant = time.Unix(1700000000, 123456789).UTC()

func BenchmarkFormatRFC3339(b *testing.B) {
	var sink string
	for b.Loop() {
		sink = RFC3339.Format(benchInstant)
	}
	_ = sink
}

func BenchmarkFormatRFC3339Nano(b *testing.B) {
	var sink string
	for b.Loop() {
		sink = RFC3339Nano.Format(benchInstant)
	}
	_ = sink
}

func BenchmarkFormatUnixMilli(b *testing.B) {
	var sink string
	for b.Loop() {
		sink = UnixMilli.Format(benchInstant)
	}
	_ = sink
}

func BenchmarkParseRFC3339(b *testing.B) {
	s := RFC3339.Format(benchInstant)
	var out time.Time
	for b.Loop() {
		out, _ = RFC3339.Parse(s)
	}
	_ = out
}

func BenchmarkParseUnixNano(b *testing.B) {
	s := UnixNano.Format(benchInstant)
	var out time.Time
	for b.Loop() {
		out, _ = UnixNano.Parse(s)
	}
	_ = out
}

func BenchmarkFormatTimeRFC3339(b *testing.B) {
	var sink any
	for b.Loop() {
		sink = FormatTime(benchInstant, RFC3339)
	}
	_ = sink
}

func BenchmarkFormatTimeUnixNano(b *testing.B) {
	var sink any
	for b.Loop() {
		sink = FormatTime(benchInstant, UnixNano)
	}
	_ = sink
}

func BenchmarkFormatDurationUnixNano(b *testing.B) {
	d := 1500 * time.Microsecond
	var sink any
	for b.Loop() {
		sink = FormatDuration(d, UnixNano)
	}
	_ = sink
}
