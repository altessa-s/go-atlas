// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package civil_test

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/time/civil"
)

func TestNewValidates(t *testing.T) {
	t.Parallel()

	_, err := civil.New(2026, time.February, 29)
	require.ErrorIs(t, err, civil.ErrInvalidDate)
	_, err = civil.New(0, time.January, 1)
	require.ErrorIs(t, err, civil.ErrInvalidDate)
	_, err = civil.New(10000, time.January, 1)
	require.ErrorIs(t, err, civil.ErrInvalidDate)
	d, err := civil.New(2028, time.February, 29)
	require.NoError(t, err)
	require.Equal(t, "2028-02-29", d.String())
}

func TestAddMonthsClamped(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		from   civil.Date
		months int
		anchor int
		want   string
	}{
		{name: "end of month to April", from: civil.MustNew(2026, time.March, 31), months: 1, anchor: 31, want: "2026-04-30"},
		{name: "end of month to February", from: civil.MustNew(2027, time.January, 31), months: 1, anchor: 31, want: "2027-02-28"},
		{name: "leap year February", from: civil.MustNew(2028, time.January, 31), months: 1, anchor: 31, want: "2028-02-29"},
		{name: "anchor restored after short month", from: civil.MustNew(2027, time.February, 28), months: 1, anchor: 31, want: "2027-03-31"},
		{name: "year rollover", from: civil.MustNew(2026, time.November, 15), months: 3, anchor: 15, want: "2027-02-15"},
		{name: "annual on leap day", from: civil.MustNew(2028, time.February, 29), months: 12, anchor: 29, want: "2029-02-28"},
		{name: "backwards", from: civil.MustNew(2026, time.March, 31), months: -1, anchor: 31, want: "2026-02-28"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, tc.from.AddMonthsClamped(tc.months, tc.anchor).String())
		})
	}
}

func TestParse(t *testing.T) {
	t.Parallel()

	d, err := civil.Parse("2026-10-05")
	require.NoError(t, err)
	require.Equal(t, civil.MustNew(2026, time.October, 5), d)

	d, err = civil.Parse("")
	require.NoError(t, err)
	require.True(t, d.IsZero())

	for _, s := range []string{"2026-02-30", "2026-1-5", "05.10.2026", "0000-00-00"} {
		_, err = civil.Parse(s)
		require.ErrorIs(t, err, civil.ErrInvalidDate, s)
	}
}

func TestOrderingAndArithmetic(t *testing.T) {
	t.Parallel()

	a, b := civil.MustNew(2026, time.January, 1), civil.MustNew(2026, time.January, 2)
	require.True(t, a.Before(b))
	require.True(t, b.After(a))
	require.Zero(t, a.Compare(a))
	require.Equal(t, -1, civil.Date{}.Compare(a), "the zero date sorts first")
	require.Equal(t, "2025-12-31", a.AddDays(-1).String())
	require.Equal(t, 365, civil.MustNew(2027, time.January, 1).DaysSince(a))
	require.Equal(t, time.Thursday, a.Weekday())
	require.Equal(t, 29, civil.DaysIn(2028, time.February))
}

func TestOfIgnoresInstantZone(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("UTC+7", 7*60*60)
	instant := time.Date(2026, time.October, 8, 20, 0, 0, 0, time.UTC) // 03:00 on Oct 9 in UTC+7
	require.Equal(t, civil.MustNew(2026, time.October, 8), civil.Of(instant))
	require.Equal(t, civil.MustNew(2026, time.October, 9), civil.Of(instant.In(loc)))
	require.Equal(t, time.Date(2026, time.October, 9, 0, 0, 0, 0, loc), civil.MustNew(2026, time.October, 9).In(loc))
}

type doc struct {
	Due  civil.Date  `json:"due"`
	Opt  civil.Date  `json:"opt"`
	Ptr  *civil.Date `json:"ptr,omitempty"`
	Note string      `json:"note"`
}

func TestJSONRoundTrip(t *testing.T) {
	t.Parallel()

	in := doc{Due: civil.MustNew(2026, time.October, 9), Note: "x"}
	b, err := json.Marshal(in)
	require.NoError(t, err)
	require.JSONEq(t, `{"due":"2026-10-09","opt":"","note":"x"}`, string(b))

	var out doc
	require.NoError(t, json.Unmarshal(b, &out))
	require.Equal(t, in, out)
}

func TestJSONAcceptsLegacyObjectForm(t *testing.T) {
	t.Parallel()

	var out doc
	raw := `{"due":{"Year":2026,"Month":10,"Day":9},"opt":{"Year":0,"Month":0,"Day":0},"ptr":null,"note":"x"}`
	require.NoError(t, json.Unmarshal([]byte(raw), &out))
	require.Equal(t, doc{Due: civil.MustNew(2026, time.October, 9), Note: "x"}, out)

	require.ErrorIs(t, json.Unmarshal([]byte(`{"due":{"Year":2026,"Month":2,"Day":30}}`), &out), civil.ErrInvalidDate)
	require.ErrorIs(t, json.Unmarshal([]byte(`{"due":"2026-02-30"}`), &out), civil.ErrInvalidDate)
	require.Error(t, json.Unmarshal([]byte(`{"due":20261009}`), &out))
}

func TestMarshalRejectsInvalidDate(t *testing.T) {
	t.Parallel()

	_, err := json.Marshal(doc{Due: civil.Date{Year: 2026, Month: time.February, Day: 30}})
	require.ErrorIs(t, err, civil.ErrInvalidDate)
}

func TestParseRejectsYearZero(t *testing.T) {
	t.Parallel()

	_, err := civil.Parse("0000-01-01")
	require.ErrorIs(t, err, civil.ErrInvalidDate, "Parse agrees with New and Valid")
	d, err := civil.Parse("0001-01-01")
	require.NoError(t, err)
	require.True(t, d.Valid())
}

func TestDaysSinceAcrossTheWholeRange(t *testing.T) {
	t.Parallel()

	first, last := civil.MustNew(1, time.January, 1), civil.MustNew(9999, time.December, 31)
	require.Equal(t, 3_652_058, last.DaysSince(first), "no time.Duration saturation")
	require.Equal(t, -3_652_058, first.DaysSince(last))
	require.Equal(t, 1, civil.MustNew(2024, time.March, 1).DaysSince(civil.MustNew(2024, time.February, 29)))
	require.Zero(t, last.DaysSince(civil.Date{}), "no date has no distance")
	require.Zero(t, civil.Date{}.DaysSince(first))
}

func TestJSONNullClearsTheDate(t *testing.T) {
	t.Parallel()

	d := civil.MustNew(2026, time.March, 15)
	require.NoError(t, json.Unmarshal([]byte(" null "), &d))
	require.True(t, d.IsZero())
}

func TestAddMonthsClampedEdges(t *testing.T) {
	t.Parallel()

	jan31 := civil.MustNew(2026, time.January, 31)
	require.Equal(t, "2026-02-01", jan31.AddMonthsClamped(1, 0).String(), "an anchor below 1 gives the 1st")
	require.Equal(t, "2026-02-01", jan31.AddMonthsClamped(1, -5).String())
	require.Equal(t, "2025-10-31", jan31.AddMonthsClamped(-3, 31).String())

	first := civil.MustNew(1, time.January, 5)
	require.True(t, first.AddMonthsClamped(-1, 1).IsZero(), "before year 1")
	require.True(t, first.AddMonthsClamped(-13, 1).IsZero())
	require.True(t, civil.MustNew(9999, time.December, 1).AddMonthsClamped(1, 1).IsZero(), "after year 9999")
	require.Equal(t, "0001-12-05", first.AddMonthsClamped(11, 5).String())
	require.True(t, jan31.AddMonthsClamped(math.MaxInt, 1).IsZero(), "no overflow")
	require.True(t, jan31.AddMonthsClamped(math.MinInt, 1).IsZero())
	require.True(t, civil.Date{}.AddMonthsClamped(1, 1).IsZero(), "no date stays no date")
	require.True(t, civil.Date{Year: 2026, Month: 13, Day: 1}.AddMonthsClamped(1, 1).IsZero(), "an invalid date gives no date")
}

func TestAddDaysEdges(t *testing.T) {
	t.Parallel()

	require.True(t, civil.Date{}.AddDays(0).IsZero(), "no date stays no date")
	require.True(t, civil.Date{}.AddDays(1).IsZero())
	require.True(t, civil.MustNew(1, time.January, 1).AddDays(-1).IsZero(), "before year 1")
	require.True(t, civil.MustNew(9999, time.December, 31).AddDays(1).IsZero(), "after year 9999")
	require.Equal(t, "9999-12-31", civil.MustNew(1, time.January, 1).AddDays(3_652_058).String())
	require.True(t, civil.MustNew(2026, time.January, 1).AddDays(math.MaxInt).IsZero(), "no overflow")
	require.Equal(t, "2024-03-01", civil.MustNew(2024, time.February, 28).AddDays(2).String())
}

func TestZeroDateHasNoInstant(t *testing.T) {
	t.Parallel()

	require.True(t, civil.Date{}.Time().IsZero())
	require.True(t, civil.Date{}.In(time.UTC).IsZero())
	require.Equal(t, "2026-10-09 00:00:00 +0000 UTC", civil.MustNew(2026, time.October, 9).Time().String())
	require.Equal(t, time.Friday, civil.MustNew(2026, time.October, 9).Weekday())
	require.Equal(t, -1, civil.Date{}.Compare(civil.MustNew(1, time.January, 1)), "no date sorts first")
}
