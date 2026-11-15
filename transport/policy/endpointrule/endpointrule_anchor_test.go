// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package endpointrule_test

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/policy/endpointrule"
)

// A pattern must match the whole endpoint: an unanchored `/admin` used to
// select the rule for "/x/admin" and "/admin/users" as well.
func TestRegistry_PatternMatchesWholeEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		pattern  string
		endpoint string
		want     bool
	}{
		{"exact", `/admin`, "/admin", true},
		{"no substring match", `/admin`, "/x/admin", false},
		{"no prefix match", `/admin`, "/admin/users", false},
		{"trailing slash is part of the endpoint", `/admin`, "/admin/", false},
		{"optional trailing slash", `/admin/?`, "/admin/", true},
		{"explicit subtree", `/admin(/.*)?`, "/admin/users", true},
		{"explicit subtree does not match a sibling", `/admin(/.*)?`, "/administrator", false},
		{"start anchor alone no longer means prefix", `^/admin`, "/admin/users", false},
		{"end anchor alone no longer means suffix", `/admin$`, "/x/admin", false},
		{"already anchored", `^/admin$`, "/admin", true},
		{"already anchored, no substring", `^/admin$`, "/x/admin", false},
		{"alternation is anchored as a whole", `/admin|/ops`, "/x/ops", false},
		{"alternation, second branch", `/admin|/ops`, "/ops", true},
		{"anchored alternation stays exact", `^/admin|/ops$`, "/admin/users", false},
		{"inline flag is kept", `(?i)/admin\..*`, "/Admin.Users/Get", true},
		{"gRPC service prefix", `/internal\..*`, "/internal.Debug/Dump", true},
		{"gRPC service prefix, other package", `/internal\..*`, "/x/internal.Debug/Dump", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var r endpointrule.Registry[rule]
			r.RegisterPattern(regexp.MustCompile(tc.pattern), &rule{name: "matched"})

			_, found := r.Lookup(tc.endpoint)
			require.Equal(t, tc.want, found)
		})
	}
}

// The caller's compiled pattern is not modified by registration.
func TestRegistry_RegisterPatternKeepsCallerPattern(t *testing.T) {
	t.Parallel()
	pattern := regexp.MustCompile(`/admin`)
	var r endpointrule.Registry[rule]
	r.RegisterPattern(pattern, &rule{})

	require.Equal(t, `/admin`, pattern.String())
	require.True(t, pattern.MatchString("/x/admin"))
}
