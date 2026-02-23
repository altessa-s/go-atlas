// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"iter"
	"slices"
	"strings"
)

// normalizeScopeValue converts a scope value (string or array) to a string slice.
// Uses normalizeScopeValueSeq internally to avoid duplicating the type switch logic.
func normalizeScopeValue(scopeValue any) ([]string, bool) {
	switch scopeValue.(type) {
	case string, []string, []any:
		return slices.Collect(normalizeScopeValueSeq(scopeValue)), true
	default:
		return nil, false
	}
}

// parseScopeClaimSeq returns an iterator that yields all scopes present in the 'scope' claim.
func parseScopeClaimSeq(claims map[string]any) iter.Seq[string] {
	scopeValue, exists := claims["scope"]
	if !exists {
		return func(yield func(string) bool) {}
	}
	return normalizeScopeValueSeq(scopeValue)
}

// normalizeScopeValueSeq converts a scope value (string or array) to an iterator.
func normalizeScopeValueSeq(scopeValue any) iter.Seq[string] {
	return func(yield func(string) bool) {
		switch v := scopeValue.(type) {
		case string:
			for _, field := range strings.Fields(v) {
				if !yield(field) {
					return
				}
			}
		case []string:
			for _, s := range v {
				if !yield(s) {
					return
				}
			}
		case []any:
			for _, raw := range v {
				if str, ok := raw.(string); ok {
					if !yield(str) {
						return
					}
				}
			}
		}
	}
}
