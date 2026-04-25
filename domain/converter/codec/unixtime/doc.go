// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package unixtime provides a codec for bidirectional conversion between
// time.Time and int64 Unix timestamps with optional millisecond precision.
//
// By default timestamps use second precision. Use [WithMilliseconds] for
// millisecond precision. Zero values can be skipped with [WithIgnoreZero].
// Both pointer and non-pointer variants are supported.
//
// Example:
//
//	codec := unixtime.New(unixtime.WithIgnoreZero())
package unixtime
