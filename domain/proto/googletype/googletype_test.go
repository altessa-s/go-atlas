// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package googletype_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/time/civil"
	"github.com/altessa-s/go-atlas/domain/proto/googletype"

	"google.golang.org/genproto/googleapis/type/date"
	"google.golang.org/genproto/googleapis/type/dayofweek"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestDateFromProto(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		d    *date.Date
		want civil.Date
	}{
		{name: "nil", d: nil},
		{name: "all zero", d: &date.Date{}},
		{name: "full date", d: &date.Date{Year: 2026, Month: 10, Day: 18}, want: civil.Date{Year: 2026, Month: time.October, Day: 18}},
		{name: "leap day", d: &date.Date{Year: 2024, Month: 2, Day: 29}, want: civil.Date{Year: 2024, Month: time.February, Day: 29}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := googletype.DateFromProto(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}

	for name, tc := range map[string]struct {
		d   *date.Date
		msg string
	}{
		"feb 30":           {&date.Date{Year: 2026, Month: 2, Day: 30}, "googletype: impossible date 2026-02-30"},
		"not a leap year":  {&date.Date{Year: 2025, Month: 2, Day: 29}, "googletype: impossible date 2025-02-29"},
		"month 13":         {&date.Date{Year: 2026, Month: 13, Day: 1}, "googletype: impossible date 2026-13-01"},
		"partial no day":   {&date.Date{Year: 2026, Month: 5}, "googletype: impossible date 2026-05-00"},
		"partial no year":  {&date.Date{Month: 5, Day: 1}, "googletype: impossible date 0000-05-01"},
		"negative day":     {&date.Date{Year: 2026, Month: 5, Day: -1}, "googletype: impossible date 2026-05--1"},
		"beyond year 9999": {&date.Date{Year: 10000, Month: 1, Day: 1}, "googletype: impossible date 10000-01-01"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := googletype.DateFromProto(tc.d)
			require.ErrorIs(t, err, googletype.ErrImpossibleDate)
			require.EqualError(t, err, tc.msg)
			require.True(t, got.IsZero())
		})
	}
}

func TestDateToProto(t *testing.T) {
	t.Parallel()

	require.Nil(t, googletype.DateToProto(civil.Date{}))
	p := googletype.DateToProto(civil.Date{Year: 2026, Month: time.October, Day: 18})
	require.Equal(t, int32(2026), p.GetYear())
	require.Equal(t, int32(10), p.GetMonth())
	require.Equal(t, int32(18), p.GetDay())

	back, err := googletype.DateFromProto(p)
	require.NoError(t, err)
	require.Equal(t, civil.Date{Year: 2026, Month: time.October, Day: 18}, back)
}

func TestWeekdays(t *testing.T) {
	t.Parallel()

	pairs := map[dayofweek.DayOfWeek]time.Weekday{
		dayofweek.DayOfWeek_MONDAY: time.Monday, dayofweek.DayOfWeek_TUESDAY: time.Tuesday,
		dayofweek.DayOfWeek_WEDNESDAY: time.Wednesday, dayofweek.DayOfWeek_THURSDAY: time.Thursday,
		dayofweek.DayOfWeek_FRIDAY: time.Friday, dayofweek.DayOfWeek_SATURDAY: time.Saturday,
		dayofweek.DayOfWeek_SUNDAY: time.Sunday,
	}
	for p, w := range pairs {
		got, err := googletype.WeekdayFromProto(p)
		require.NoError(t, err)
		require.Equal(t, w, got)
		require.Equal(t, p, googletype.WeekdayToProto(w))
	}

	for _, bad := range []dayofweek.DayOfWeek{dayofweek.DayOfWeek_DAY_OF_WEEK_UNSPECIFIED, 8, -1} {
		_, err := googletype.WeekdayFromProto(bad)
		require.ErrorIs(t, err, googletype.ErrUnknownWeekday)
	}
	require.Equal(t, dayofweek.DayOfWeek_DAY_OF_WEEK_UNSPECIFIED, googletype.WeekdayToProto(7))
	require.Equal(t, dayofweek.DayOfWeek_DAY_OF_WEEK_UNSPECIFIED, googletype.WeekdayToProto(-1))
}

func TestWeekdayLists(t *testing.T) {
	t.Parallel()

	days, err := googletype.WeekdaysFromProto([]dayofweek.DayOfWeek{dayofweek.DayOfWeek_SUNDAY, dayofweek.DayOfWeek_MONDAY})
	require.NoError(t, err)
	require.Equal(t, []time.Weekday{time.Sunday, time.Monday}, days)
	require.Equal(t, []dayofweek.DayOfWeek{dayofweek.DayOfWeek_SUNDAY, dayofweek.DayOfWeek_MONDAY}, googletype.WeekdaysToProto(days))

	none, err := googletype.WeekdaysFromProto(nil)
	require.NoError(t, err)
	require.Nil(t, none)
	require.Empty(t, googletype.WeekdaysToProto(nil))

	_, err = googletype.WeekdaysFromProto([]dayofweek.DayOfWeek{dayofweek.DayOfWeek_MONDAY, dayofweek.DayOfWeek_DAY_OF_WEEK_UNSPECIFIED})
	require.ErrorIs(t, err, googletype.ErrUnknownWeekday)
	ie, ok := errors.AsType[*googletype.IndexError](err)
	require.True(t, ok, "err = %v", err)
	require.Equal(t, 1, ie.Index)
	require.EqualError(t, err, "element 1: googletype: unknown or unspecified day of the week: DAY_OF_WEEK_UNSPECIFIED")
}

func TestDurations(t *testing.T) {
	t.Parallel()

	got, err := googletype.DurationFromProto(nil)
	require.NoError(t, err)
	require.Zero(t, got)

	got, err = googletype.DurationFromProto(durationpb.New(45 * time.Second))
	require.NoError(t, err)
	require.Equal(t, 45*time.Second, got)

	for name, d := range map[string]*durationpb.Duration{
		"mixed signs":     {Seconds: 1, Nanos: -1},
		"nanos too large": {Nanos: 1_000_000_000},
		"beyond range":    {Seconds: 315_576_000_001},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := googletype.DurationFromProto(d)
			require.ErrorIs(t, err, googletype.ErrInvalidDuration)
		})
	}

	sat, err := googletype.DurationFromProto(&durationpb.Duration{Seconds: 315_576_000_000})
	require.NoError(t, err)
	require.Equal(t, time.Duration(math.MaxInt64), sat, "a valid duration beyond time.Duration saturates")

	require.Nil(t, googletype.DurationToProto(0))
	require.Equal(t, 30*time.Second, googletype.DurationToProto(30*time.Second).AsDuration())
	require.Equal(t, -time.Second, googletype.DurationToProto(-time.Second).AsDuration())
}

func TestTimestampToProto(t *testing.T) {
	t.Parallel()

	require.Nil(t, googletype.TimestampToProto(time.Time{}))
	now := time.Date(2026, 10, 18, 12, 30, 0, 5, time.UTC)
	require.True(t, googletype.TimestampToProto(now).AsTime().Equal(now))
	require.NotNil(t, googletype.TimestampToProto(time.Unix(0, 0)), "the Unix epoch is not the zero time")
}
