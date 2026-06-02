// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package fieldmask provides hierarchical field mask utilities for Protocol
// Buffers.
//
// A [FieldMask] is a nested map representing dot-separated field paths. It
// supports filtering ([FieldMask.Filter]), pruning ([FieldMask.Prune]), and set
// operations ([FieldMask.Union], [FieldMask.Intersection], [FieldMask.Difference])
// on protobuf messages. For update-style semantics, [FieldMask.ApplyUpdateMask]
// validates google.api.field_behavior annotations (REQUIRED, IMMUTABLE,
// OUTPUT_ONLY, IDENTIFIER) before applying the mask. IDENTIFIER fields are
// treated like IMMUTABLE in the update path: per AIP-203 the identifier names
// the resource and must not be modified by an update.
//
// Well-known types [structpb.Struct], [structpb.ListValue], and [structpb.Value]
// are handled transparently — their dynamic keys are navigable by mask operations.
//
// Masks can be created from dot-separated paths ([FromPaths]),
// [fieldmaskpb.FieldMask] ([FromProtoFieldMask]), or by introspecting a message
// schema ([FromMessage], [FromSetFields]).
//
// # Request extraction
//
// [ExtractUpdateMask], [ExtractReadMask], and [SetUpdateMask] locate the
// conventional AIP-134 update_mask / AIP-157 read_mask fields on a request
// message via descriptor reflection. They power the
// transport/grpc/interceptors/fieldmask interceptor and stay gRPC-unaware so
// non-gRPC callers can reuse them.
//
// [UpdateExtractorFunc] and [ReadExtractorFunc] are the transport-agnostic
// function shapes the gRPC interceptor talks to. Callers whose requests do
// not match the canonical AIP-134 shape (for example, mask nested inside an
// Options sub-message) build a custom extractor through
// [NewUpdateExtractor] / [NewReadExtractor] from typed getters and pass it
// to the interceptor via WithUpdateExtractor / WithReadExtractor.
// [DefaultUpdateExtractor] and [DefaultReadExtractor] wrap the reflection
// helpers in the same function shape and serve as the interceptor's
// built-in fallback.
//
// Example:
//
//	mask := fieldmask.FromPaths("user.name", "user.address.city")
//	mask.Filter(protoMessage)
//
//	combined := mask1.Union(mask2)
//	common := mask1.Intersection(mask2)
package fieldmask
