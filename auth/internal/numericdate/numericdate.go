// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package numericdate

import (
	"cmp"
	"encoding/json"
	"math"
	"math/big"
	"time"
)

const (
	decimalBase      = 10
	int64Digits      = 19
	nanosecondDigits = 9
)

// Date stores integral Unix seconds inline. Only a sub-nanosecond remainder
// needs a decimal. Its sign is relative to the nearest clock tick.
type Date struct {
	seconds   int64
	nanos     int64
	remainder *decimal
}

// FromTime preserves the clock's full resolution.
func FromTime(t time.Time) Date {
	return Date{seconds: t.Unix(), nanos: int64(t.Nanosecond())}
}

// Parse accepts signed JSON dates and the numeric types used by in-process callers.
func Parse(value any) (Date, bool) {
	switch v := value.(type) {
	case int:
		return Date{seconds: int64(v)}, true
	case int64:
		return Date{seconds: v}, true
	case float64:
		if !(v >= math.MinInt64 && v < -float64(math.MinInt64)) {
			return Date{}, false
		}
		if math.Trunc(v) == v {
			return Date{seconds: int64(v)}, true
		}
		// A binary fraction has a terminating decimal with at most as
		// many places as the exponent of its power-of-two denominator.
		r := new(big.Rat).SetFloat64(v)
		return parseDate(json.Number(r.FloatString(r.Denom().BitLen() - 1)))
	case json.Number:
		s := v.String()
		if len(s) == 0 || (s[0] != '-' && (s[0] < '0' || s[0] > '9')) {
			return Date{}, false
		}
		if n, err := v.Int64(); err == nil {
			digits := s
			if digits[0] == '-' {
				digits = digits[1:]
			}
			return Date{seconds: n}, len(digits) == 1 || digits[0] != '0'
		}
		return parseDate(v)
	}
	return Date{}, false
}

func parseDate(n json.Number) (Date, bool) {
	d, ok := parseDecimal(n)
	if !ok || d.sign == 0 {
		return Date{}, ok
	}
	if d.point.Cmp(big.NewInt(int64Digits)) > 0 {
		return Date{}, false
	}
	if d.point.Cmp(big.NewInt(-nanosecondDigits)) < 0 {
		// Its magnitude is below 0.1 ns. Retain it without expanding the exponent.
		d.point.Add(&d.point, big.NewInt(nanosecondDigits))
		return Date{remainder: &d}, true
	}
	point := int(d.point.Int64())
	var whole uint64
	for i := range point {
		whole = whole*decimalBase + d.digit(i)
	}
	limit := uint64(math.MaxInt64)
	if d.sign < 0 {
		limit++
	}
	if whole > limit || (whole == limit && len(d.digits) > point) {
		return Date{}, false
	}

	var nanos int64
	for i := point; i < point+nanosecondDigits; i++ {
		nanos = nanos*decimalBase + int64(d.digit(i))
	}
	residual, roundAway := d.subNanosecond(point + nanosecondDigits)
	if roundAway {
		nanos++
	}
	// time.Unix normalizes negative nanoseconds and a carry of one second.
	t := time.Unix(int64(whole)*int64(d.sign), nanos*int64(d.sign))
	date := FromTime(t)
	date.remainder = residual
	return date, true
}

// CompareOffset compares d to start + offset, including sub-nanosecond
// remainders. Only the bounded duration is split into seconds and nanoseconds.
func (d Date) CompareOffset(start Date, offset time.Duration) int {
	seconds := int64(offset / time.Second)
	nanos := start.nanos + int64(offset%time.Second)
	if nanos < 0 {
		seconds--
		nanos += int64(time.Second)
	} else if nanos >= int64(time.Second) {
		seconds++
		nanos -= int64(time.Second)
	}
	if seconds > 0 && start.seconds > math.MaxInt64-seconds {
		return -1
	}
	if seconds < 0 && start.seconds < math.MinInt64-seconds {
		return 1
	}
	if c := cmp.Compare(d.seconds, start.seconds+seconds); c != 0 {
		return c
	}
	if c := cmp.Compare(d.nanos, nanos); c != 0 {
		return c
	}
	switch {
	case d.remainder == nil && start.remainder == nil:
		return 0
	case d.remainder == nil:
		return -start.remainder.sign
	case start.remainder == nil:
		return d.remainder.sign
	default:
		return d.remainder.compare(start.remainder)
	}
}

// Time returns the UTC instant rounded down to the clock's resolution.
func (d Date) Time() time.Time {
	if d.remainder != nil && d.remainder.sign < 0 {
		return time.Unix(d.seconds, d.nanos-1).UTC()
	}
	return time.Unix(d.seconds, d.nanos).UTC()
}

// Expiry rounds up to the first representable instant at or after the date.
func (d Date) Expiry() time.Time {
	if d.remainder != nil && d.remainder.sign > 0 {
		return time.Unix(d.seconds, d.nanos+1).UTC()
	}
	return time.Unix(d.seconds, d.nanos).UTC()
}

// Seconds reports whether the date is exactly representable as an int64.
func (d Date) Seconds() (int64, bool) {
	return d.seconds, d.nanos == 0 && d.remainder == nil
}
