// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package timeformat provides time formatting with RFC3339 and Unix
// timestamp support. Each [Format] is an enum value that selects one of
// the supported representations and exposes Format/Parse entry points
// for converting between [time.Time] and the wire representation.
//
// # Usage
//
//	formatted := timeformat.RFC3339.Format(time.Now())
//	parsed, _ := timeformat.UnixMilli.Parse("1704110400123")
//
// # Supported formats
//
// The package ships six formats: [RFC3339] and [RFC3339Nano] produce
// strings, while [Unix], [UnixMilli], [UnixMicro], and [UnixNano] produce
// int64 values. [FormatTime] returns the matching any-typed result for a
// given format, which is convenient when the format is selected at
// runtime.
//
// # Duration support
//
// [FormatDuration] renders a [time.Duration] using the same rules. For
// the integer formats the result is in nanoseconds so that the value can
// round-trip losslessly through the sub-second variants.
package timeformat
