// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package timeformat

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
			require.NoError(t, err, "Parse (out=%q)", out)

			// Round-trip preserves only the precision of the chosen format.
			switch f {
			case RFC3339, Unix:
				require.Equal(t, in.Unix(), parsed.Unix())
			case RFC3339Nano, UnixNano:
				require.Equal(t, in.UnixNano(), parsed.UnixNano())
			case UnixMilli:
				require.Equal(t, in.UnixMilli(), parsed.UnixMilli())
			case UnixMicro:
				require.Equal(t, in.UnixMicro(), parsed.UnixMicro())
			default:
				require.Failf(t, "unhandled format", "%q", f)
			}
		})
	}
}

func TestFormatTime_Types(t *testing.T) {
	t.Parallel()

	now := time.Unix(1700000000, 0)

	_, ok := FormatTime(now, RFC3339).(string)
	require.True(t, ok, "RFC3339 should format to string")
	_, ok = FormatTime(now, Unix).(int64)
	require.True(t, ok, "Unix should format to int64")
	_, ok = FormatTime(now, UnixMilli).(int64)
	require.True(t, ok, "UnixMilli should format to int64")
	_, ok = FormatTime(now, UnixMicro).(int64)
	require.True(t, ok, "UnixMicro should format to int64")
	_, ok = FormatTime(now, UnixNano).(int64)
	require.True(t, ok, "UnixNano should format to int64")
}

func TestFormatDuration_UnixMicroIsNanoseconds(t *testing.T) {
	t.Parallel()

	d := 1500 * time.Microsecond // 1_500_000 ns
	got := FormatDuration(d, UnixMicro)
	ns, ok := got.(int64)
	require.True(t, ok, "expected int64 for UnixMicro duration, got %T", got)
	require.Equal(t, d.Nanoseconds(), ns)
}
