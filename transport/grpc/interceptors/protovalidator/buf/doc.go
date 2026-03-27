// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package buf provides helpers for working with buf protovalidate validation
// errors in gRPC interceptors.
//
// It offers:
//   - [BuildValidator]: creates a proto message validator function that validates
//     messages using [protovalidate.Validator], converts violations into structured
//     [protovalidatev1.BadRequest] details attached to gRPC status responses, and
//     generates human-readable error messages via [BuildValidationError].
//   - [BuildErrorCode]: derives error codes from rule IDs and field paths
//     (e.g., "required" on field "userName" produces "USER_NAME_REQUIRED").
//   - [BuildValidationFilter]: returns a [protovalidate.FilterFunc] controlling
//     which messages are validated (currently allows all).
//   - [BuildValidationError]: joins all violations into a single formatted error
//     string ("validation failed: field: message [rule]; ...").
//
// The validator is lazily initialized as a singleton via [sync.Once]. Violation
// details include field path components with map key and repeated index support,
// mapped to [protovalidatev1.FieldPathComponent].
package buf
