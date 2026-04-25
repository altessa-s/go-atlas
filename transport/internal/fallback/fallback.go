// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fallback

import (
	"github.com/altessa-s/go-atlas/core/runtime/panics"
)

// Behavior controls how a middleware or interceptor reacts when an internal
// operation (e.g. rate-limit lookup, auth token validation) fails. The zero
// value is an empty string and is not valid; use [ParseBehavior] or one of
// the named constants.
type Behavior string

const (
	// Allow continues processing on error (maximum availability).
	// The request proceeds even if the middleware operation fails.
	Allow Behavior = "allow"

	// Deny rejects the request on error (maximum protection).
	// The request is blocked if the middleware operation fails.
	Deny Behavior = "deny"

	// Error returns an error for explicit handling by the caller.
	// The middleware operation failure is propagated to the handler.
	Error Behavior = "error"
)

// String returns the string representation of the Behavior.
func (b Behavior) String() string {
	return string(b)
}

// ShouldAllow returns true if the behavior is Allow.
func (b Behavior) ShouldAllow() bool {
	return b == Allow
}

// ShouldDeny returns true if the behavior is Deny.
func (b Behavior) ShouldDeny() bool {
	return b == Deny
}

// ShouldReturnError returns true if the behavior is Error.
func (b Behavior) ShouldReturnError() bool {
	return b == Error
}

// IsValid returns true if the Behavior has a valid value.
//
// Example:
//
//	fallback.BehaviorAllow.IsValid() // true
//	fallback.Behavior("unknown").IsValid() // false
func (b Behavior) IsValid() bool {
	switch b {
	case Allow, Deny, Error:
		return true
	default:
		return false
	}
}

// MustValidate panics if the Behavior is not valid.
//
// Example:
//
//	fallback.BehaviorAllow.MustValidate() // no panic
//	fallback.Behavior("bad").MustValidate() // panics
func (b Behavior) MustValidate() {
	panics.Must(b.IsValid(), "invalid fallback behavior: "+string(b))
}

// ParseBehavior converts a raw string (e.g. from configuration) to a
// [Behavior]. Unrecognized values default to [Deny] so that
// misconfiguration fails closed rather than open.
func ParseBehavior(s string) Behavior {
	switch Behavior(s) {
	case Allow:
		return Allow
	case Deny:
		return Deny
	case Error:
		return Error
	default:
		return Deny
	}
}
