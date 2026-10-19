// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package googletype

import (
	"fmt"
	"time"

	"github.com/altessa-s/go-atlas/core/time/civil"

	"google.golang.org/genproto/googleapis/type/date"
	"google.golang.org/genproto/googleapis/type/dayofweek"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// DateFromProto converts a google.type.Date to a civil date. A nil or
// all-zero date is the zero civil.Date (absent). Any other date must be a
// full calendar date in years 1–9999 (civil.Date.Valid): an impossible or
// partial one (Feb 30, a zero month or day) wraps [ErrImpossibleDate], so it
// never round-trips through storage as the zero date.
func DateFromProto(d *date.Date) (civil.Date, error) {
	if d == nil || (d.GetYear() == 0 && d.GetMonth() == 0 && d.GetDay() == 0) {
		return civil.Date{}, nil
	}
	cd := civil.Date{Year: int(d.GetYear()), Month: time.Month(d.GetMonth()), Day: int(d.GetDay())}
	if !cd.Valid() {
		return civil.Date{}, fmt.Errorf("%w %04d-%02d-%02d", ErrImpossibleDate, d.GetYear(), d.GetMonth(), d.GetDay())
	}
	return cd, nil
}

// DateToProto converts a civil date to a google.type.Date; the zero date is
// nil (absent).
func DateToProto(d civil.Date) *date.Date {
	if d.IsZero() {
		return nil
	}
	return &date.Date{Year: int32(d.Year), Month: int32(d.Month), Day: int32(d.Day)} //nolint:gosec // civil dates fit in int32
}

// WeekdayFromProto maps a google.type.DayOfWeek (MONDAY=1 … SUNDAY=7) to a
// time.Weekday (Sunday=0). UNSPECIFIED or an undefined value wraps
// [ErrUnknownWeekday].
func WeekdayFromProto(d dayofweek.DayOfWeek) (time.Weekday, error) {
	switch {
	case d == dayofweek.DayOfWeek_SUNDAY:
		return time.Sunday, nil
	case d >= dayofweek.DayOfWeek_MONDAY && d <= dayofweek.DayOfWeek_SATURDAY:
		return time.Weekday(d), nil
	default:
		return 0, fmt.Errorf("%w: %v", ErrUnknownWeekday, d)
	}
}

// WeekdaysFromProto maps a repeated google.type.DayOfWeek with
// [WeekdayFromProto]; an offending element is reported as an [*IndexError]
// wrapping its error. An empty list is nil.
func WeekdaysFromProto(days []dayofweek.DayOfWeek) ([]time.Weekday, error) {
	if len(days) == 0 {
		return nil, nil
	}
	out := make([]time.Weekday, 0, len(days))
	for i, d := range days {
		w, err := WeekdayFromProto(d)
		if err != nil {
			return nil, &IndexError{Index: i, Err: err}
		}
		out = append(out, w)
	}
	return out, nil
}

// WeekdayToProto maps a time.Weekday to a google.type.DayOfWeek; a value
// outside Sunday–Saturday is DAY_OF_WEEK_UNSPECIFIED.
func WeekdayToProto(w time.Weekday) dayofweek.DayOfWeek {
	switch {
	case w == time.Sunday:
		return dayofweek.DayOfWeek_SUNDAY
	case w >= time.Monday && w <= time.Saturday:
		return dayofweek.DayOfWeek(w)
	default:
		return dayofweek.DayOfWeek_DAY_OF_WEEK_UNSPECIFIED
	}
}

// WeekdaysToProto maps weekdays with [WeekdayToProto].
func WeekdaysToProto(days []time.Weekday) []dayofweek.DayOfWeek {
	out := make([]dayofweek.DayOfWeek, 0, len(days))
	for _, d := range days {
		out = append(out, WeekdayToProto(d))
	}
	return out
}

// DurationFromProto converts an optional google.protobuf.Duration: nil is
// zero (the caller's default). One that fails durationpb.CheckValid (nanos
// out of range or of the other sign than seconds, beyond ±10000 years) wraps
// [ErrInvalidDuration]. A valid duration beyond the time.Duration range
// saturates, as durationpb.AsDuration does.
func DurationFromProto(d *durationpb.Duration) (time.Duration, error) {
	if d == nil {
		return 0, nil
	}
	if err := d.CheckValid(); err != nil {
		return 0, fmt.Errorf("%w: %w", ErrInvalidDuration, err)
	}
	return d.AsDuration(), nil
}

// DurationToProto converts a duration; zero (the default) is nil (absent).
func DurationToProto(d time.Duration) *durationpb.Duration {
	if d == 0 {
		return nil
	}
	return durationpb.New(d)
}

// TimestampToProto converts a time; the zero time is nil (absent).
func TimestampToProto(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}
