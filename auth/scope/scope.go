// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scope

import (
	"errors"
	"slices"
	"strings"
)

// Scope is an authorization scope identifier, e.g. "files:read". Scopes are
// opaque strings; their structure (the ":" delimiter and so on) matters only to
// a [Matcher] that chooses to interpret it.
type Scope = string

// ErrAccessDenied is returned by [Enforcer.Enforce] when a principal may not
// perform an action. It is transport-neutral: a gRPC adapter maps it to
// codes.PermissionDenied, an HTTP adapter to 403. Matchable with errors.Is.
var ErrAccessDenied = errors.New("scope: access denied")

// Matcher reports whether the granted scopes satisfy a required scope. It is
// the pluggable comparison strategy used by [ScopeAuthorizer]; pick [Exact] for
// flat equality or [Wildcard] for hierarchical grants.
type Matcher func(granted []Scope, required Scope) bool

// Exact returns a [Matcher] that grants access only when required is present in
// granted verbatim. It is the default, predictable strategy and the right
// choice unless a service deliberately issues hierarchical wildcard grants.
func Exact() Matcher {
	return slices.Contains
}

// Wildcard returns a [Matcher] that honors hierarchical grants delimited by sep.
// A grant of "*" satisfies every required scope; a grant ending in sep+"*" (for
// example "files:*" with sep ":") satisfies any required scope under that
// prefix ("files:read", "files:write:bulk"). Exact membership still matches.
// The prefix boundary is respected, so "files:*" does not satisfy "filesx:read".
func Wildcard(sep string) Matcher {
	suffix := sep + "*"
	return func(granted []Scope, required Scope) bool {
		for _, g := range granted {
			if g == required || g == "*" {
				return true
			}
			// A matching grant is prefix+sep+"*", so g[:len(g)-1] is
			// prefix+sep without allocating.
			if strings.HasSuffix(g, suffix) && strings.HasPrefix(required, g[:len(g)-1]) {
				return true
			}
		}
		return false
	}
}
