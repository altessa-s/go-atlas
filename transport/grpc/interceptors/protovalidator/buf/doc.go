// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package bufhelpers provides helpers for working with buf protovalidate validation
// errors in gRPC interceptors.
//
// It offers:
//   - [BuildValidator]: creates a proto message validator function that validates
//     messages using [protovalidate.Validator], converts violations into structured
//     [protovalidatev1.BadRequest] details attached to gRPC status responses, and
//     generates human-readable error messages via [BuildValidationError]. Pass
//     [WithResolver] to supply a service's own reason-code catalog.
//   - [BuildErrorCode]: maps a violation's rule ID and field path to a canonical,
//     client-facing reason code — never the raw rule ID. "required" on field
//     "userName" produces "USER_NAME_REQUIRED"; standard rules map to registry
//     codes (e.g. "int64.gte" -> "INVALID_MIN_LENGTH_OR_VALUE"); unknown or
//     uncatalogued rules become "UNKNOWN". See package
//     [github.com/altessa-s/go-atlas/transport/grpc/interceptors/protovalidator/reasoncode].
//   - [BuildValidationFilter]: returns a [protovalidate.FilterFunc] controlling
//     which messages are validated (currently allows all).
//   - [BuildValidationError]: joins all violations into a single formatted error
//     string ("validation failed: field: message [rule]; ...").
//
// The validator is lazily initialized as a singleton via [sync.Once]. Violation
// details include field path components with map key and repeated index support,
// mapped to [protovalidatev1.FieldPathComponent].
package bufhelpers
