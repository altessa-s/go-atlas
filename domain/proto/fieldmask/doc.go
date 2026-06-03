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
// [FieldMask.ApplyUpdateMask] also rejects indexed access to repeated fields
// (`authors.0`, `authors.0.given_name`) as [ValidationError] per AIP-161:
// update masks address whole repeated fields, never an individual element.
// Read paths ([FieldMask.Filter], [FieldMask.Prune], [FieldMask.Validate])
// tolerate the same segments — AIP-161 lets the implementation ignore them
// on read.
//
// Map keys that contain a dot or other non-identifier characters are
// addressed by wrapping the key in backticks per AIP-161, for example
// `reviews.` + "`John Smith`" or `metadata.` + "`google.com/project`".
// The path parser ([FromPaths], [FieldMask.Contains], [FieldMask.Validate])
// treats a backtick-quoted run as a single segment; [FieldMask.ToPaths]
// re-quotes keys that contain a dot so the round-trip is lossless.
//
// The AIP-161 wildcard segment "*" ([WildcardSegment]) matches every
// element of a repeated field or every value of a map field. It is
// honored by [FieldMask.Filter], [FieldMask.Prune], [FieldMask.Validate]
// and [FieldMask.ApplyUpdateMask]: a leaf wildcard ("aliases.*") keeps
// (or, for [FieldMask.Prune], clears) the whole collection; a branch
// wildcard ("aliases.*.display_name") applies the nested sub-mask to
// every element / value. For maps a specific-key entry wins over the
// wildcard for matched keys. [FieldMask.ApplyUpdateMask] strips
// OUTPUT_ONLY fields transitively through wildcard sub-masks before
// writing the cleaned mask back.
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
// AIP-161 deprecates carrying the read mask on the request message and
// directs callers to AIP-157, which transports the mask through a side
// channel (HTTP "$fields" query, gRPC "x-goog-fieldmask" metadata).
// [MetadataReadExtractor] reads that metadata header and returns a mask in
// the same shape. [ChainReadExtractors] and [ChainUpdateExtractors]
// combine multiple extractors and return the first one that signals
// ok=true — the gRPC interceptor's default read extractor is the chain
// (metadata, request-field) so modern services work out of the box while
// legacy request-field deployments keep functioning unchanged.
//
// Example:
//
//	mask := fieldmask.FromPaths("user.name", "user.address.city")
//	mask.Filter(protoMessage)
//
//	combined := mask1.Union(mask2)
//	common := mask1.Intersection(mask2)
package fieldmask
