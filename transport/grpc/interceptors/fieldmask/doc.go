// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package fieldmask is a gRPC server interceptor that wires
// [github.com/altessa-s/go-aip/fieldmask] and its update and extract packages
// into the gRPC chain.
//
// By default, a request mask is a singular google.protobuf.FieldMask declared
// as update_mask / read_mask, falling back to updateMask / readMask only when
// the corresponding snake_case name is not declared. A non-empty
// [WithMaskFieldName] selects an exact declared name without fallback.
//
// Updates use update.Prepare to validate raw paths, normalize the resource, write back the initial effective mask, and retain a deferred plan.
// IMMUTABLE and IDENTIFIER values are captured in the deferred plan for comparison with stored state;
// their paths do not authorize writes in the initial mask.
// Protected paths, including explicit clears, are deferred. An unchanged protected value is ignored; a change is INVALID_ARGUMENT from stateful
// planning in the handler. OUTPUT_ONLY input is sanitized during normalization, including explicit empty and dropped-only masks.
// An omitted mask implies populated fields; an explicit empty mask is a no-op. An empty interim mask can still require protected-field comparison.
// With explicit paths or an explicit empty mask, the handler-visible resource retains selected input and the resource routing identity.
// Unselected containers are dropped even when they contain IMMUTABLE or nested IDENTIFIER fields.
// Selected collection elements retain their identifiers, map keys, and list positions; OUTPUT_ONLY input is cleared.
// The plan captures original values for deferred comparison; selected protected values remain visible. Omitted masks and * retain protected input.
// An omitted mask selects populated leaves; an explicit empty mask clears unselected mutable input and performs no writes or secret changes.
// A dropped-only mask also filters unselected input. Writable masks retain selected values for downstream validation; * processes replacement selection.
// OUTPUT_ONLY is cleared in every case. The deferred plan captures original input for comparison, replacements and selected INPUT_ONLY secrets.
// No KeepIdentity cleanup runs before completion. Never persist the normalized request as a replacement.
//
// The handler must call update.Patch.Run with the received context and stored-resource callbacks, or call Spec.Plan with
// update.WithPlanContext(ctx) and update.WithCurrent(stored), then merge the final paths. Merging only the written-back mask does not perform
// the protected-field check. Always complete the plan, even when the written-back mask is empty.
//
// Unary handlers receive the plan through update.ContextWithPlan. Streaming handlers obtain a context snapshot from stream.Context() after
// each successful receive. Each message gets its own plan; use a fresh resource instance when retaining earlier contexts. Both interceptors share
// a per-message tracker: the first successful normalization and its original mask are retained, and the second interceptor reuses that plan.
//
// On classified read methods (Get*, List*, Search*, BatchGet*) the
// interceptor extracts the read_mask from the request before the handler runs
// and applies [fieldmask.FieldMask.Filter] to the successful response so
// fields outside the mask are pruned before serialization. A malformed
// read_mask is rejected with InvalidArgument before the handler runs, and a
// path the response schema does not hold with InvalidArgument after it. The handler can
// read the same mask with [ReadMaskFromContext] and push it down to storage
// as a projection.
//
// # AIP-157 metadata read mask
//
// [MetadataReadExtractor] reads the read mask from incoming gRPC metadata
// (default key [DefaultMetadataReadMaskHeader], "x-goog-fieldmask";
// override with [WithMetadataHeader]). The built-in read path chains it
// before the request-field [extract.DefaultRead], and callers that replace
// the read extractor can compose it themselves through [extract.ChainRead].
//
// # Method classification
//
// The short method name (the part after the final "/") is matched
// case-sensitively against AIP prefixes:
//
//	Update*, Patch*, BatchUpdate*   -> KindUpdate  (PreCall: update.Prepare + writeback)
//	Get*, List*, Search*, BatchGet* -> KindRead    (PostCall: Filter response)
//	everything else                 -> KindNone   (passthrough)
//
// Create* is intentionally not handled — AIP-133 does not define a mask on
// the create path. Use [WithMethodKind] to override the classification for a
// specific fully-qualified method.
//
// WithMaskFieldName also controls the field name in strict validation error
// details.
package fieldmask
