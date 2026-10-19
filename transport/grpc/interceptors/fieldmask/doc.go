// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package fieldmask is a gRPC server interceptor that wires
// [github.com/altessa-s/go-atlas/domain/proto/fieldmask] into the gRPC chain.
//
// On classified Update* methods it locates the AIP-134 update_mask and the
// sibling resource sub-message via descriptor reflection, runs
// [fieldmask.FieldMask.ApplyUpdateMask] over the pair, and writes the
// OUTPUT_ONLY-cleaned mask back onto the request before the handler runs.
// Behavior violations (REQUIRED cleared, IMMUTABLE modified, IDENTIFIER
// named in the mask) are converted to InvalidArgument with an
// io.altessa.badrequest.v1.BadRequest detail attached — one FieldViolation per
// entry, the same detail type the protovalidate interceptor emits, so
// [github.com/altessa-s/go-atlas/transport/grpc/client.ParseError] reads both.
//
// An IDENTIFIER set in the resource body addresses the resource (AIP-203): it
// is kept, is not an error, and is left out of the writeback mask and of the
// mask synthesized by [WithApplyEmptyUpdateMask], so the handler sees only the
// fields the client asked to change. The same holds for an IDENTIFIER inside a
// populated singular sub-message (profile.id). A mask naming such a
// sub-message whole ("profile") is rejected, populated or absent: it would
// authorize replacing or clearing the identifier with its parent. The same
// applies to an IMMUTABLE descendant; the synthesized mask names the set
// fields of such a sub-message instead of the whole sub-message. A path into
// collection elements naming an element IDENTIFIER or IMMUTABLE field
// ("labels.k.id", "aliases.*.id") is rejected too. The synthesis and the
// identifier cleanup are [fieldmask.FromSetFieldsForUpdate] and
// [fieldmask.FieldMask.DropIdentifiers].
//
// On classified read methods (Get*, List*, Search*, BatchGet*) the
// interceptor extracts the read_mask from the request before the handler runs
// and applies [fieldmask.FieldMask.Filter] to the successful response so
// fields outside the mask are pruned before serialization. The handler can
// read the same mask with [ReadMaskFromContext] and push it down to storage
// as a projection.
//
// # Method classification
//
// The short method name (the part after the final "/") is matched
// case-sensitively against AIP prefixes:
//
//	Update*, Patch*, BatchUpdate*   -> KindUpdate  (PreCall: ApplyUpdateMask + writeback)
//	Get*, List*, Search*, BatchGet* -> KindRead    (PostCall: Filter response)
//	everything else                 -> KindNone   (passthrough)
//
// Create* is intentionally not handled — AIP-133 does not define a mask on
// the create path. Use [WithMethodKind] to override the classification for a
// specific fully-qualified method.
//
//	package fieldmask
package fieldmask
