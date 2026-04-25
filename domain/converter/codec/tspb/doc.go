// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package tspb provides a codec for bidirectional conversion between
// timestamppb.Timestamp and time.Time or int64 Unix timestamps.
//
// By default int64 values use second precision; use [WithMilliseconds] for
// millisecond precision. Zero-value timestamps can be skipped with [WithIgnoreZero].
// Both pointer and non-pointer variants are supported.
//
// Uses unsafe pointer arithmetic for zero-allocation reads and writes when the
// underlying reflect.Value is addressable; falls back to standard reflect otherwise.
//
// Example:
//
//	codec := tspb.New(tspb.WithIgnoreZero(), tspb.WithMilliseconds())
package tspb
