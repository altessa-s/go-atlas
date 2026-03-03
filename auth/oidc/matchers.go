// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"slices"
	"strings"
)

const (
	// smallSetThreshold is the threshold for linear vs map-based lookup optimization.
	smallSetThreshold = 3
)

// ClaimEquals creates a matcher checking if a string claim equals the expected value.
// Returns false if the claim is missing or not a string.
//
// Example:
//
//	matcher := oidc.ClaimEquals("token_type", "service")
func ClaimEquals(claimName, expectedValue string) PresetMatcherFunc {
	return func(claims map[string]any) bool {
		value, exists := claims[claimName]
		if !exists {
			return false
		}

		strValue, ok := value.(string)
		if !ok {
			return false
		}

		return strValue == expectedValue
	}
}

// ClaimContains creates a matcher checking if a string claim contains a substring.
// Returns false if the claim is missing or not a string.
//
// Example:
//
//	matcher := oidc.ClaimContains("realm", "one2work")
func ClaimContains(claimName, substring string) PresetMatcherFunc {
	return func(claims map[string]any) bool {
		value, exists := claims[claimName]
		if !exists {
			return false
		}

		strValue, ok := value.(string)
		if !ok {
			return false
		}

		return strings.Contains(strValue, substring)
	}
}

// ClaimExists creates a matcher checking if a claim is present in the token.
//
// Example:
//
//	matcher := oidc.ClaimExists("email")
func ClaimExists(claimName string) PresetMatcherFunc {
	return func(claims map[string]any) bool {
		_, exists := claims[claimName]
		return exists
	}
}

// HasScope creates a matcher checking if the token contains a specific scope.
// Supports scope as space-separated string or array of strings.
//
// Example:
//
//	matcher := oidc.HasScope("read:users")
func HasScope(requiredScope string) PresetMatcherFunc {
	return func(claims map[string]any) bool {
		for s := range parseScopeClaimSeq(claims) {
			if s == requiredScope {
				return true
			}
		}
		return false
	}
}

// HasAnyScope creates a matcher checking if the token has at least one of the scopes.
// Supports scope as space-separated string or array of strings.
//
// The required scope set is pre-built once at construction time, avoiding
// per-call map allocations regardless of set size.
//
// Example:
//
//	matcher := oidc.HasAnyScope("read:users", "write:users", "admin")
func HasAnyScope(requiredScopes ...string) PresetMatcherFunc {
	// Pre-build required set once at construction time.
	requiredSet := make(map[string]struct{}, len(requiredScopes))
	for _, s := range requiredScopes {
		requiredSet[s] = struct{}{}
	}

	return func(claims map[string]any) bool {
		// Small sets: linear scan avoids map lookup overhead.
		if len(requiredScopes) <= smallSetThreshold {
			for s := range parseScopeClaimSeq(claims) {
				if slices.Contains(requiredScopes, s) {
					return true
				}
			}
			return false
		}

		// Larger sets: O(1) lookup from pre-built set (zero allocation).
		for s := range parseScopeClaimSeq(claims) {
			if _, ok := requiredSet[s]; ok {
				return true
			}
		}
		return false
	}
}

// HasAllScopes creates a matcher checking if the token has all specified scopes.
// Supports scope as space-separated string or array of strings.
//
// Uses a bitmask to track matched scopes, avoiding per-call map allocations.
// Supports up to 64 unique required scopes (sufficient for all practical use cases).
//
// Example:
//
//	matcher := oidc.HasAllScopes("read:users", "write:users")
func HasAllScopes(requiredScopes ...string) PresetMatcherFunc {
	// Pre-build a bit-indexed lookup: each unique required scope gets a bit position.
	requiredIndex := make(map[string]uint64, len(requiredScopes))
	var allBits uint64
	for i, s := range requiredScopes {
		if i >= 64 {
			// Fallback for >64 required scopes (practically impossible).
			return hasAllScopesFallback(requiredScopes)
		}
		bit := uint64(1) << i
		if _, dup := requiredIndex[s]; !dup {
			requiredIndex[s] = bit
			allBits |= bit
		}
	}

	return func(claims map[string]any) bool {
		var seen uint64
		for s := range parseScopeClaimSeq(claims) {
			if bit, ok := requiredIndex[s]; ok {
				seen |= bit
				if seen == allBits {
					return true
				}
			}
		}
		return false
	}
}

// hasAllScopesFallback handles the edge case of >64 required scopes using map tracking.
func hasAllScopesFallback(requiredScopes []string) PresetMatcherFunc {
	requiredSet := make(map[string]struct{}, len(requiredScopes))
	for _, s := range requiredScopes {
		requiredSet[s] = struct{}{}
	}

	return func(claims map[string]any) bool {
		matched := make(map[string]struct{}, len(requiredSet))
		for s := range parseScopeClaimSeq(claims) {
			if _, required := requiredSet[s]; required {
				matched[s] = struct{}{}
				if len(matched) == len(requiredSet) {
					return true
				}
			}
		}
		return len(matched) >= len(requiredSet)
	}
}

// MatcherAnd creates a matcher returning true only if all matchers return true.
//
// Example:
//
//	matcher := oidc.MatcherAnd(oidc.ClaimEquals("token_type", "service"), oidc.HasScope("admin"))
func MatcherAnd(matchers ...PresetMatcherFunc) PresetMatcherFunc {
	return func(claims map[string]any) bool {
		for _, matcher := range matchers {
			if !matcher(claims) {
				return false
			}
		}
		return true
	}
}

// MatcherOr creates a matcher returning true if any matcher returns true.
//
// Example:
//
//	matcher := oidc.MatcherOr(oidc.ClaimEquals("role", "admin"), oidc.ClaimEquals("role", "mod"))
func MatcherOr(matchers ...PresetMatcherFunc) PresetMatcherFunc {
	return func(claims map[string]any) bool {
		for _, matcher := range matchers {
			if matcher(claims) {
				return true
			}
		}
		return false
	}
}

// MatcherNot creates a matcher that inverts the result of another matcher.
//
// Example:
//
//	matcher := oidc.MatcherNot(oidc.ClaimEquals("token_type", "service"))
func MatcherNot(matcher PresetMatcherFunc) PresetMatcherFunc {
	return func(claims map[string]any) bool {
		return !matcher(claims)
	}
}

// ClientIDEquals creates a matcher checking if client_id (or azp fallback) equals the value.
//
// Example:
//
//	matcher := oidc.ClientIDEquals("service-backend")
func ClientIDEquals(expectedClientID string) PresetMatcherFunc {
	return func(claims map[string]any) bool {
		clientID, exists := claims["client_id"]
		if !exists {
			// Fallback to azp
			clientID, exists = claims["azp"]
			if !exists {
				return false
			}
		}

		strValue, ok := clientID.(string)
		if !ok {
			return false
		}

		return strValue == expectedClientID
	}
}

// IssuerEquals creates a matcher checking if the iss claim equals the expected value.
//
// Example:
//
//	matcher := oidc.IssuerEquals("https://auth.example.com")
func IssuerEquals(expectedIssuer string) PresetMatcherFunc {
	return ClaimEquals("iss", expectedIssuer)
}

// AudienceContains creates a matcher checking if aud contains the expected value.
// Supports aud as a string or array of strings.
//
// Example:
//
//	matcher := oidc.AudienceContains("api.example.com")
func AudienceContains(expectedAudience string) PresetMatcherFunc {
	return func(claims map[string]any) bool {
		audValue, exists := claims["aud"]
		if !exists {
			return false
		}

		switch v := audValue.(type) {
		case string:
			return v == expectedAudience
		case []any:
			for _, aud := range v {
				if strAud, ok := aud.(string); ok && strAud == expectedAudience {
					return true
				}
			}
		}

		return false
	}
}
