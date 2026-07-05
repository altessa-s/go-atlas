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
//     [WithReasonCode] to attach a service-defined, client-facing reason code to
//     each field violation.
//   - [WithReasonCode]: supplies a [ReasonCoder] mapping a rule ID and field name
//     to a canonical reason code. go-atlas ships no built-in mapping — the set of
//     codes is part of a service's public error contract, so the service owns it;
//     when unset, no code is emitted.
//   - [BuildValidationFilter]: returns a [protovalidate.FilterFunc] controlling
//     which messages are validated (currently allows all).
//   - [BuildValidationError]: joins all violations into a single formatted error
//     string ("validation failed: field: message [rule]; ...").
//
// The validator is lazily initialized as a singleton via [sync.Once]. Violation
// details include field path components with map key and repeated index support,
// mapped to [protovalidatev1.FieldPathComponent].
package bufhelpers
