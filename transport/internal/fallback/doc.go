// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package fallback defines three error-handling strategies for middlewares
// and interceptors:
//
//   - [Allow] -- let the request proceed despite the error (maximum availability).
//   - [Deny]  -- reject the request on error (maximum protection).
//   - [Error] -- propagate the error to the caller for explicit handling.
//
// [ParseBehavior] converts an untrusted string to a [Behavior], defaulting
// to [Deny] for unrecognized values so that misconfiguration fails closed.
//
// Example:
//
//	behavior := fallback.Allow
//	if behavior.ShouldAllow() {
//	    // continue processing despite error
//	}
package fallback
