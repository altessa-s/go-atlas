// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package fieldbehavior is a gRPC server interceptor that runs the matching
// [github.com/altessa-s/go-atlas/domain/proto/fieldbehavior] Strip* function
// against the request and the response based on the gRPC method name.
//
// # Method classification
//
// The short method name (the part after the final "/") is matched
// case-sensitively against AIP-133/AIP-134 prefixes:
//
//	Create*, BatchCreate*   -> StripCreate on the request payload
//	Update*, Patch*,
//	BatchUpdate*            -> StripUpdate on the request payload
//	everything else         -> request is not stripped
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
// The interceptor always runs in mutation mode. Strict mode (returning a
// [fieldbehavior.BehaviorViolationError] instead of mutating) is not exposed
// here because rejecting the wire payload at the interceptor layer is a
// contract decision that belongs in the handler.
//
//	package fieldbehavior
package fieldbehavior
