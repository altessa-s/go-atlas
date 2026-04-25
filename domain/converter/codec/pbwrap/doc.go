// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package pbwrap provides a codec for bidirectional conversion between
// protobuf wrapper types (wrapperspb.*) and primitive Go types.
//
// Supported wrappers: StringValue, Int32Value, Int64Value, UInt32Value,
// UInt64Value, BoolValue, FloatValue, DoubleValue. BytesValue is excluded
// because its []byte nature requires special handling.
//
// Example:
//
//	codec := pbwrap.New(pbwrap.WithIgnoreZeroValues())
//	converter.Convert(src, &dst, converter.WithCodecs(codec))
package pbwrap
