// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package timeformat

import (
	"fmt"
	"strconv"
	"time"
)

// Format defines a time formatting strategy for serializing and deserializing
// timestamps. Supported strategies include RFC 3339 text layouts and numeric
// Unix epoch representations at various precisions. The zero value is not a
// valid format; use one of the predefined constants such as [RFC3339] or [UnixMilli].
type Format string

const (
	// RFC3339 selects the RFC 3339 layout "2006-01-02T15:04:05Z07:00" with
	// second precision. It is the default fallback used by [Format.Format] and
	// [Format.Parse] when an unrecognized [Format] value is encountered.
	RFC3339 Format = "rfc3339"

	// RFC3339Nano selects the RFC 3339 layout with nanosecond precision:
	// "2006-01-02T15:04:05.999999999Z07:00".
	RFC3339Nano Format = "rfc3339nano"

	// Unix selects a numeric representation of seconds elapsed since
	// 1970-01-01T00:00:00Z. [Format.Format] returns a decimal string;
	// [FormatTime] returns an int64.
	Unix Format = "unix"

	// UnixMilli selects a numeric representation of milliseconds elapsed since
	// 1970-01-01T00:00:00Z. [Format.Format] returns a decimal string;
	// [FormatTime] returns an int64.
	UnixMilli Format = "unixmilli"

	// UnixMicro selects a numeric representation of microseconds elapsed since
	// 1970-01-01T00:00:00Z. [Format.Format] returns a decimal string;
	// [FormatTime] returns an int64.
	UnixMicro Format = "unixmicro"

	// UnixNano selects a numeric representation of nanoseconds elapsed since
	// 1970-01-01T00:00:00Z. [Format.Format] returns a decimal string;
	// [FormatTime] returns an int64.
	UnixNano Format = "unixnano"
)

// String returns the lowercase identifier of the [Format] (e.g. "rfc3339", "unixmilli"),
// satisfying the [fmt.Stringer] interface.
func (f Format) String() string {
	return string(f)
}

// Format renders t as a string according to the receiver's formatting strategy.
// For RFC layouts the result is a text timestamp; for Unix variants it is a
// decimal integer string. Unrecognized [Format] values fall back to [RFC3339].
func (f Format) Format(t time.Time) string {
	switch f {
	case RFC3339:
		return t.Format(time.RFC3339)
	case RFC3339Nano:
		return t.Format(time.RFC3339Nano)
	case Unix:
		return strconv.FormatInt(t.Unix(), 10)
	case UnixMilli:
		return strconv.FormatInt(t.UnixMilli(), 10)
	case UnixMicro:
		return strconv.FormatInt(t.UnixMicro(), 10)
	case UnixNano:
		return strconv.FormatInt(t.UnixNano(), 10)
	default:
		return t.Format(time.RFC3339)
	}
}

// Parse interprets s as a timestamp in the receiver's format and returns the
// corresponding [time.Time]. For RFC layouts the string must match the layout
// exactly; for Unix variants it must be a valid base-10 integer. An error is
// returned if s cannot be parsed. Unrecognized [Format] values fall back to
// parsing with the [RFC3339] layout.
func (f Format) Parse(s string) (time.Time, error) {
	switch f {
	case RFC3339:
		return time.Parse(time.RFC3339, s)
	case RFC3339Nano:
		return time.Parse(time.RFC3339Nano, s)
	case Unix:
		sec, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid Unix timestamp: %w", err)
		}
		return time.Unix(sec, 0), nil
	case UnixMilli:
		ms, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid UnixMilli timestamp: %w", err)
		}
		return time.UnixMilli(ms), nil
	case UnixMicro:
		us, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid UnixMicro timestamp: %w", err)
		}
		return time.UnixMicro(us), nil
	case UnixNano:
		ns, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid UnixNano timestamp: %w", err)
		}
		return time.Unix(0, ns), nil
	default:
		return time.Parse(time.RFC3339, s)
	}
}

// FormatTime formats t using the given [Format] and returns a type appropriate
// for structured logging or JSON serialization. For RFC layouts the result is a
// string; for Unix variants it is an int64 at the corresponding precision.
// Unrecognized format values fall back to [RFC3339] and return a string.
//
// Unlike [Format.Format], which always returns a string, FormatTime preserves
// the native numeric type for Unix-based formats so that encoders can emit a
// JSON number rather than a quoted string.
func FormatTime(t time.Time, format Format) any {
	switch format {
	case RFC3339:
		return t.Format(time.RFC3339)
	case RFC3339Nano:
		return t.Format(time.RFC3339Nano)
	case Unix:
		return t.Unix()
	case UnixMilli:
		return t.UnixMilli()
	case UnixMicro:
		return t.UnixMicro()
	case UnixNano:
		return t.UnixNano()
	default:
		return t.Format(time.RFC3339)
	}
}

// FormatDuration formats d using the given [Format] and returns a type suitable
// for structured logging or JSON serialization. For Unix-based formats ([Unix],
// [UnixMilli], [UnixMicro], [UnixNano]) the result is an int64 of nanoseconds.
// For RFC layouts and any unrecognized format the [time.Duration] value is
// returned as-is, allowing the encoder to apply its own duration rendering.
func FormatDuration(d time.Duration, format Format) any {
	switch format {
	case UnixNano, UnixMicro, UnixMilli, Unix:
		// For Unix formats, return duration as nanoseconds
		return d.Nanoseconds()
	default:
		// For RFC formats, return duration as is
		return d
	}
}
