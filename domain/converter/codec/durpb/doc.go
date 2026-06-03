// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package durpb provides a codec for bidirectional conversion between
// durationpb.Duration and time.Duration.
//
// Zero-value durations can be skipped with [WithIgnoreZero]. Both pointer and
// non-pointer variants are supported.
//
// time.Duration <-> int64 is not handled here: time.Duration has an int64 kind,
// so the converter's built-in convertible path maps it without a codec.
//
// Uses unsafe pointer arithmetic for zero-allocation reads and writes when the
// underlying reflect.Value is addressable; falls back to standard reflect otherwise.
//
// Unlike [durationpb.Duration.AsDuration], out-of-range Seconds are not clamped to
// the representable time.Duration range; values that originated as a time.Duration
// round-trip exactly, but a Duration with Seconds beyond ~292 years will overflow.
//
// Example:
//
//	codec := durpb.New(durpb.WithIgnoreZero())
package durpb
