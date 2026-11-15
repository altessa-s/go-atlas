// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package fieldbehavior is a gRPC server interceptor that runs the matching
// [github.com/altessa-s/go-aip/fieldbehavior] Strip* function
// against the request and the response based on the gRPC method name.
//
// By default, the Update mask is a singular google.protobuf.FieldMask declared
// as update_mask, falling back to updateMask only when update_mask is not declared.
// [WithUpdateNormalizer] can override this lookup through the supplied normalizer.
//
// # Method classification
//
// The short method name (the part after the final "/") is matched
// case-sensitively against AIP-133/AIP-134 prefixes:
//
//	Create*, BatchCreate*   -> StripCreate on the request payload
//	Update*, Patch*,
//	BatchUpdate*            -> the update_mask applied to the resource
//	everything else         -> request is not stripped
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
// When lookup fails, StripUpdate clears OUTPUT_ONLY and retains protected
// values. The same handling applies outside the resource. Formats are checked
// on all retained values. These fallback/outside values require handler checks.
//
// The response of every method whose handler returns successfully is passed
// through StripResponse, so INPUT_ONLY fields (passwords, one-shot tokens)
// never leak through the read path. Disable that with [WithSkipResponse].
//
// # Overrides
//
// Use [WithMethodKind] to override the classification for a fully-qualified
// method name, including the leading slash:
//
//	fieldbehavior.WithMethodKind("/x.v1.X/ImportBuckets", fieldbehavior.KindCreate)
//	fieldbehavior.WithMethodKind("/x.v1.X/RotateKey",     fieldbehavior.KindSkip)
//
// # Strict mode
//
// The interceptor clears the fields it may clear. Strict mode (reporting a
// populated OUTPUT_ONLY or IDENTIFIER field as a
// [github.com/altessa-s/go-aip/fieldbehavior.BehaviorViolationError]
// instead of clearing it) is not exposed here because rejecting the wire
// payload at the interceptor layer is a contract decision that belongs in the
// handler.
//
//	package fieldbehavior
package fieldbehavior
