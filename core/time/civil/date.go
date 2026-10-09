// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package civil

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrInvalidDate is returned when a value is not a valid calendar date in
// years 1–9999.
var ErrInvalidDate = errors.New("civil: invalid calendar date")

const (
	// layout is the ISO 8601 calendar date layout; its lexical order is the
	// date order.
	layout = "2006-01-02"
	// minYear and maxYear bound the years with a four-digit text form.
	minYear = 1
	maxYear = 9999
	// monthsPerYear is the number of months in a year.
	monthsPerYear = 12
	// secondsPerDay is the length of a calendar day in UTC.
	secondsPerDay = 24 * 60 * 60
	// maxDaySpan bounds AddDays offsets: no larger step stays in range.
	maxDaySpan = 3_652_059
	// maxMonthSpan bounds AddMonthsClamped offsets the same way.
	maxMonthSpan = maxYear * monthsPerYear
)

// Date is a calendar date without a time zone. The zero value is "no date".
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// New returns the date year-month-day, or [ErrInvalidDate] when it is not a
// real calendar date in years 1–9999.
func New(year int, month time.Month, day int) (Date, error) {
	d := Date{Year: year, Month: month, Day: day}
	if !d.Valid() {
		return Date{}, fmt.Errorf("%w: %04d-%02d-%02d", ErrInvalidDate, year, int(month), day)
	}
	return d, nil
}

// MustNew is like [New] but panics on an invalid date. Use it for constants
// and tests.
func MustNew(year int, month time.Month, day int) Date {
	d, err := New(year, month, day)
	if err != nil {
		panic(err)
	}
	return d
}

// Parse parses "YYYY-MM-DD". The empty string yields the zero Date; any
// other malformed value, or a date outside years 1–9999, fails with
// [ErrInvalidDate].
func Parse(s string) (Date, error) {
	if s == "" {
		return Date{}, nil
	}
	t, err := time.Parse(layout, s)
	if err != nil {
		return Date{}, fmt.Errorf("%w: %q", ErrInvalidDate, s)
	}
	return New(t.Year(), t.Month(), t.Day())
}

// Of returns the calendar date of t in t's location.
func Of(t time.Time) Date {
	y, m, d := t.Date()
	return Date{Year: y, Month: m, Day: d}
}

// Today returns the current date in loc.
func Today(loc *time.Location) Date {
	return Of(time.Now().In(loc))
}

// DaysIn returns the number of days in month m of year y.
func DaysIn(y int, m time.Month) int {
	return time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// IsZero reports whether d is the zero ("no date") value.
func (d Date) IsZero() bool { return d == Date{} }

// Valid reports whether d is a real calendar date in years 1–9999. The zero
// Date is not valid.
func (d Date) Valid() bool {
	if d.Year < minYear || d.Year > maxYear || d.Month < time.January || d.Month > time.December || d.Day < 1 {
		return false
	}
	return d.Day <= DaysIn(d.Year, d.Month)
}

// String formats d as "YYYY-MM-DD", or "" for the zero value.
func (d Date) String() string {
	if d.IsZero() {
		return ""
	}
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, int(d.Month), d.Day)
}

// In returns midnight of d in loc, or the zero [time.Time] for the zero
// Date.
func (d Date) In(loc *time.Location) time.Time {
	if d.IsZero() {
		return time.Time{}
	}
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, loc)
}

// Time returns midnight UTC of d, or the zero [time.Time] for the zero Date.
func (d Date) Time() time.Time { return d.In(time.UTC) }

// Weekday returns the day of the week of d. The zero Date has none; its
// result is meaningless — check [Date.IsZero] first.
func (d Date) Weekday() time.Weekday { return d.Time().Weekday() }

// Compare returns -1, 0 or +1 as d is before, equal to or after o. The zero
// Date sorts before every date.
func (d Date) Compare(o Date) int {
	switch {
	case d.Year != o.Year:
		return cmpInt(d.Year, o.Year)
	case d.Month != o.Month:
		return cmpInt(int(d.Month), int(o.Month))
	default:
		return cmpInt(d.Day, o.Day)
	}
}

// Before reports whether d is before o.
func (d Date) Before(o Date) bool { return d.Compare(o) < 0 }

// After reports whether d is after o.
func (d Date) After(o Date) bool { return d.Compare(o) > 0 }

// AddDays returns d shifted by n days. The zero Date, or an invalid one,
// yields the zero Date, as does a result outside years 1–9999.
func (d Date) AddDays(n int) Date {
	if !d.Valid() || n > maxDaySpan || n < -maxDaySpan {
		return Date{}
	}
	return inRange(Of(d.Time().AddDate(0, 0, n)))
}

// DaysSince returns the number of days from o to d, negative when d is
// before o; 0 when either is the zero Date.
func (d Date) DaysSince(o Date) int {
	if d.IsZero() || o.IsZero() {
		return 0
	}
	// Unix seconds, not Time.Sub: a time.Duration spans only ~292 years.
	return int((d.Time().Unix() - o.Time().Unix()) / secondsPerDay)
}

// inRange returns d, or the zero Date when d falls outside years 1–9999.
func inRange(d Date) Date {
	if d.Year < minYear || d.Year > maxYear {
		return Date{}
	}
	return d
}

// AddMonthsClamped returns the date n months after d with day of month
// anchorDay, clamped to the month: anchor 31 gives 30 April and 28 or 29
// February, an anchor below 1 gives the 1st. Billing schedules keep their
// anchor day across short months this way, unlike [time.Time.AddDate],
// which overflows into the next month. The zero Date, or an invalid one,
// yields the zero Date, as does a result outside years 1–9999.
func (d Date) AddMonthsClamped(n, anchorDay int) Date {
	if !d.Valid() || n > maxMonthSpan || n < -maxMonthSpan {
		return Date{}
	}
	total := d.Year*monthsPerYear + int(d.Month) - 1 + n
	if total < minYear*monthsPerYear || total >= (maxYear+1)*monthsPerYear {
		return Date{}
	}
	y, m := total/monthsPerYear, time.Month(total%monthsPerYear+1)
	return Date{Year: y, Month: m, Day: max(1, min(anchorDay, DaysIn(y, m)))}
}

// MarshalText implements [encoding.TextMarshaler]: "YYYY-MM-DD", or "" for
// the zero Date. An invalid non-zero date fails with [ErrInvalidDate].
func (d Date) MarshalText() ([]byte, error) {
	if !d.IsZero() && !d.Valid() {
		return nil, fmt.Errorf("%w: %04d-%02d-%02d", ErrInvalidDate, d.Year, int(d.Month), d.Day)
	}
	return []byte(d.String()), nil
}

// UnmarshalText implements [encoding.TextUnmarshaler] with [Parse].
func (d *Date) UnmarshalText(b []byte) error {
	v, err := Parse(string(b))
	if err != nil {
		return err
	}
	*d = v
	return nil
}

// legacyDate is the object form encoding/json produces for a date struct
// without text methods.
type legacyDate struct {
	Year  int
	Month time.Month
	Day   int
}

// UnmarshalJSON accepts a "YYYY-MM-DD" string, "" or null for the zero
// Date, and the legacy object form {"Year":…,"Month":…,"Day":…}. An object
// holding a non-zero invalid date fails with [ErrInvalidDate].
func (d *Date) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	switch {
	case bytes.Equal(b, []byte("null")):
		*d = Date{}
		return nil
	case len(b) > 0 && b[0] == '{':
		var l legacyDate
		if err := json.Unmarshal(b, &l); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidDate, err)
		}
		v := Date(l)
		if !v.IsZero() && !v.Valid() {
			return fmt.Errorf("%w: %04d-%02d-%02d", ErrInvalidDate, v.Year, int(v.Month), v.Day)
		}
		*d = v
		return nil
	default:
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidDate, err)
		}
		return d.UnmarshalText([]byte(s))
	}
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
