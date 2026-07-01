// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package reasoncode maps protovalidate rule IDs to canonical, client-facing
// validation reason codes.
//
// A reason code is part of a service's public error contract, so it should be a
// stable code rather than the raw protovalidate rule ID (a dotted,
// implementation-specific identifier such as "int64.gte"). This package owns all
// code formation: the standard protovalidate rules are mapped here, and
// service-specific rules are supplied by the caller as a catalog passed to
// [NewResolver].
//
// # Resolution
//
// Most rules map from the rule ID alone ([Resolver.Resolve]). Some need more
// context — "required" needs the field name, and combined numeric range rules
// need the field value. For those the caller passes a [Violation] to
// [Resolver.ResolveViolation], which knows how to read that context but not how
// to extract it (extraction is the caller's job). A rule that resolves to no
// known code yields [Unknown], so a rule ID never reaches the client.
//
// # Usage
//
//	resolver := reasoncode.NewResolver(catalog, "acme.")
//	code := resolver.ResolveViolation(v) // e.g. "INVALID_MIN_LENGTH_OR_VALUE"
package reasoncode
