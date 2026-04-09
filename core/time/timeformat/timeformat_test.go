// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package timeformat

import (
	"testing"
	"time"
)

func TestFormatParse_RoundTripInstant(t *testing.T) {
	t.Parallel()

	// Use a fixed instant to avoid flakiness.
	in := time.Unix(1700000000, 123456789).UTC()

	formats := []Format{RFC3339, RFC3339Nano, Unix, UnixMilli, UnixMicro, UnixNano}
	for _, f := range formats {
		t.Run(f.String(), func(t *testing.T) {
			t.Parallel()
			out := f.Format(in)
			parsed, err := f.Parse(out)
			if err != nil {
				t.Fatalf("Parse err=%v (out=%q)", err, out)
			}

			// Round-trip preserves only the precision of the chosen format.
			switch f {
			case RFC3339, Unix:
				if parsed.Unix() != in.Unix() {
					t.Fatalf("parsed unix=%d, want %d", parsed.Unix(), in.Unix())
				}
			case RFC3339Nano, UnixNano:
				if parsed.UnixNano() != in.UnixNano() {
					t.Fatalf("parsed ns=%d, want %d", parsed.UnixNano(), in.UnixNano())
				}
			case UnixMilli:
				if parsed.UnixMilli() != in.UnixMilli() {
					t.Fatalf("parsed ms=%d, want %d", parsed.UnixMilli(), in.UnixMilli())
				}
			case UnixMicro:
				if parsed.UnixMicro() != in.UnixMicro() {
					t.Fatalf("parsed us=%d, want %d", parsed.UnixMicro(), in.UnixMicro())
				}
			default:
				t.Fatalf("unhandled format %q", f)
			}
		})
	}
}

func TestFormatTime_Types(t *testing.T) {
	t.Parallel()

	now := time.Unix(1700000000, 0)

	if _, ok := FormatTime(now, RFC3339).(string); !ok {
		t.Fatalf("RFC3339 should format to string")
	}
	if _, ok := FormatTime(now, Unix).(int64); !ok {
		t.Fatalf("Unix should format to int64")
	}
	if _, ok := FormatTime(now, UnixMilli).(int64); !ok {
		t.Fatalf("UnixMilli should format to int64")
	}
	if _, ok := FormatTime(now, UnixMicro).(int64); !ok {
		t.Fatalf("UnixMicro should format to int64")
	}
	if _, ok := FormatTime(now, UnixNano).(int64); !ok {
		t.Fatalf("UnixNano should format to int64")
	}
}

func TestFormatDuration_UnixMicroIsNanoseconds(t *testing.T) {
	t.Parallel()

	d := 1500 * time.Microsecond // 1_500_000 ns
	got := FormatDuration(d, UnixMicro)
	ns, ok := got.(int64)
	if !ok {
		t.Fatalf("expected int64 for UnixMicro duration, got %T", got)
	}
	if ns != d.Nanoseconds() {
		t.Fatalf("ns=%d, want %d", ns, d.Nanoseconds())
	}
}
