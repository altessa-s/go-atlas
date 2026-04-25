// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package convcodec provides core types for custom conversion codecs and a chain execution engine.
//
// Individual codec implementations live in subpackages:
//
//   - [github.com/altessa-s/go-atlas/domain/converter/codec/pbwrap] — protobuf wrapper ↔ primitive conversion
//   - [github.com/altessa-s/go-atlas/domain/converter/codec/oneof] — struct ↔ protobuf oneof conversion
//   - [github.com/altessa-s/go-atlas/domain/converter/codec/mapslice] — map keys/values → slice conversion
//   - [github.com/altessa-s/go-atlas/domain/converter/codec/unixtime] — time.Time ↔ int64 Unix timestamp conversion
//   - [github.com/altessa-s/go-atlas/domain/converter/codec/jsonpb] — json.RawMessage ↔ structpb.Struct conversion
//   - [github.com/altessa-s/go-atlas/domain/converter/codec/tspb] — timestamppb.Timestamp ↔ time.Time / int64 conversion
//
// Example usage:
//
//	codecSet := convcodec.NewCodecsSet(pbwrap.New(), unixtime.New())
//	codecSet.Run("fieldName", srcVal, dstVal, defaultHandler)
package convcodec
