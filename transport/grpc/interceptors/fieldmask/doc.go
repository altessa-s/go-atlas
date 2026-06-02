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
// touched) are converted to InvalidArgument with a google.rpc.BadRequest
// detail attached — one FieldViolation per entry, the canonical AIP error
// shape.
//
// On classified read methods (Get*, List*, Search*, BatchGet*) the
// interceptor extracts the read_mask from the request before the handler runs
// and applies [fieldmask.FieldMask.Filter] to the successful response so
// fields outside the mask are pruned before serialization.
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
