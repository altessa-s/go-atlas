// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package middlewareconfig

import ozzo_rules "github.com/altessa-s/ozzo-rules"

// FallbackBehavior defines how interceptors behave when encountering errors
// or when storage backends are unavailable. Used by rate limiting, idempotency,
// and other interceptors that require graceful degradation.
type FallbackBehavior string

const (
	// FallbackBehaviorAllow allows all requests when the underlying service fails.
	// This provides maximum availability but no protection.
	// Use for non-critical services where availability is more important than protection.
	FallbackBehaviorAllow FallbackBehavior = "allow"

	// FallbackBehaviorDeny rejects all requests when the underlying service fails.
	// This provides maximum protection but may impact availability.
	// Use for critical services where protection is essential for stability.
	FallbackBehaviorDeny FallbackBehavior = "deny"

	// FallbackBehaviorError returns an error when the underlying service fails.
	// This is useful for debugging and testing, but not recommended for production.
	FallbackBehaviorError FallbackBehavior = "error"
)

// AllFallbackBehaviors returns all valid FallbackBehavior values.
func AllFallbackBehaviors() []FallbackBehavior {
	return []FallbackBehavior{
		FallbackBehaviorAllow,
		FallbackBehaviorDeny,
		FallbackBehaviorError,
	}
}

// FallbackBehaviorRule returns the validation rule accepting exactly the
// FallbackBehavior values, for schemas that expose a fallbackBehavior field.
func FallbackBehaviorRule() ozzo_rules.OneOfRule[FallbackBehavior] {
	return ozzo_rules.OneOf(AllFallbackBehaviors()...)
}
