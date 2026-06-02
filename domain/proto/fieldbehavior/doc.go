// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package fieldbehavior strips fields from a [proto.Message] based on its
// google.api.field_behavior annotations.
//
// The package solves a different problem than [fieldmask]: where fieldmask
// validates that a FieldMask does not reference immutable or output-only paths,
// fieldbehavior walks the message payload itself and clears (or rejects)
// fields whose behavior makes them invalid for the current request kind.
//
// Three AIP-203 use cases are supported out of the box:
//
//   - [StripCreate] on a Create-request resource — clears OUTPUT_ONLY and
//     IDENTIFIER fields the client should not be supplying.
//   - [StripUpdate] on an Update-request resource — additionally clears
//     IMMUTABLE fields.
//   - [StripResponse] on a server response — clears INPUT_ONLY fields
//     (typically secrets) that must never leave the server.
//
// Use [Strip] with [WithBehaviors] for any custom combination. Pass
// [WithStrict] to surface populated stripped fields as a
// [*BehaviorViolationError] instead of silently clearing them.
//
// Example:
//
//	if err := fieldbehavior.StripCreate(req.GetResource()); err != nil {
//	    return nil, err
//	}
//
//	resp, err := s.repo.Get(ctx, id)
//	if err != nil { return nil, err }
//	if err := fieldbehavior.StripResponse(resp); err != nil {
//	    return nil, err
//	}
package fieldbehavior
