// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package timeformat_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/time/timeformat"
)

// formats is every strategy the package offers, paired with the precision it
// keeps. A round trip can only preserve what the format encodes: a Unix-second
// layout has nothing to say about nanoseconds.
var formats = []struct {
	format    timeformat.Format
	precision time.Duration
}{
	{timeformat.RFC3339, time.Second},
	{timeformat.RFC3339Nano, time.Nanosecond},
	{timeformat.Unix, time.Second},
	{timeformat.UnixMilli, time.Millisecond},
	{timeformat.UnixMicro, time.Microsecond},
	{timeformat.UnixNano, time.Nanosecond},
}

// FuzzFormatParseRoundTrip pins that each format reads back what it wrote, to
// the precision it claims.
//
// These formats are what timestamps are stored and logged in, so a format/parse
// pair that disagrees corrupts records rather than failing loudly. The
// interesting inputs are the ones a table forgets: instants before the epoch,
// sub-second remainders that round the wrong way, and the far ends of the range
// where a numeric layout overflows.
func FuzzFormatParseRoundTrip(f *testing.F) {
	f.Add(int64(0), int64(0))
	f.Add(int64(1700000000), int64(123456789))
	f.Add(int64(-1), int64(999999999))          // Just before the epoch.
	f.Add(int64(-6795364578), int64(871345152)) // Near time.Time's zero value.
	f.Add(int64(253402300799), int64(0))        // Year 9999.

	f.Fuzz(func(t *testing.T, sec, nsec int64) {
		// Keep the instant inside the range every layout can express. RFC 3339
		// has a four-digit year and the nanosecond layouts overflow an int64
		// past ~year 2262; a value outside that says nothing about the code.
		if sec < -62135596800 || sec > 253402300799 {
			t.Skip("outside the range shared by every layout")
		}
		instant := time.Unix(sec, ((nsec%1e9)+1e9)%1e9).UTC()
		if instant.Year() > 2262 || instant.Year() < 1678 {
			t.Skip("outside the int64 nanosecond range")
		}

		for _, tc := range formats {
			t.Run(string(tc.format), func(t *testing.T) {
				rendered := tc.format.Format(instant)

				parsed, err := tc.format.Parse(rendered)
				require.NoError(t, err, "%s could not read back what it wrote: %q", tc.format, rendered)

				require.Equal(t, instant.Truncate(tc.precision).UTC(), parsed.Truncate(tc.precision).UTC(),
					"%s round trip lost the instant: rendered %q", tc.format, rendered)
			})
		}
	})
}

// FuzzParseRejectsGarbage covers the other direction: arbitrary text.
//
// Parse is what turns a stored or received string back into a timestamp, so it
// meets whatever the store or the peer holds — including values written by an
// older version, or by something else entirely. Anything it accepts must
// re-render to the same text, or a value silently changes meaning as it passes
// through.
func FuzzParseRejectsGarbage(f *testing.F) {
	f.Add("rfc3339", "2023-11-14T22:13:20Z")
	f.Add("unix", "1700000000")
	f.Add("unixmilli", "-1")
	f.Add("unixnano", "99999999999999999999")
	f.Add("rfc3339", "")
	f.Add("nonsense", "1700000000")
	f.Add("unix", "1e10")
	f.Add("unix", "+0")

	f.Fuzz(func(t *testing.T, formatName, input string) {
		format := timeformat.Format(formatName)

		parsed, err := format.Parse(input)
		if err != nil {
			require.True(t, parsed.IsZero(), "a rejected input must not also yield an instant")
			return
		}

		// An accepted value has to survive being written back out. A parser
		// that is more permissive than its formatter turns one timestamp into
		// two spellings, and a store that round-trips values ends up with both.
		require.Equal(t, format.Format(parsed), format.Format(mustReparse(t, format, format.Format(parsed))),
			"%s is not stable across a second round trip of %q", format, input)
	})
}

// mustReparse parses text that this package just produced; failing to do so is
// the assertion, not an error to handle.
func mustReparse(t *testing.T, format timeformat.Format, text string) time.Time {
	t.Helper()

	parsed, err := format.Parse(text)
	require.NoError(t, err, "%s could not read back its own output %q", format, text)

	return parsed
}
