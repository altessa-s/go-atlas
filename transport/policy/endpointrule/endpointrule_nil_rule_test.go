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

type nilRuleTestRule struct{ name string }

// A nil rule used to be stored and reported as found, so the first matching
// request dereferenced it. Registration refuses it instead.
func TestRegistry_NilRuleIsRefusedAtRegistration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		register func(*endpointrule.Registry[nilRuleTestRule])
	}{
		{"Register", func(r *endpointrule.Registry[nilRuleTestRule]) { r.Register("/healthz", nil) }},
		{"RegisterPattern", func(r *endpointrule.Registry[nilRuleTestRule]) {
			r.RegisterPattern(regexp.MustCompile(`/admin/.*`), nil)
		}},
		{"RegisterEndpoints", func(r *endpointrule.Registry[nilRuleTestRule]) { r.RegisterEndpoints(nil, "/a", "/b") }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var reg endpointrule.Registry[nilRuleTestRule]

			require.Panics(t, func() { tc.register(&reg) })

			for _, endpoint := range []string{"/healthz", "/admin/users", "/a", "/b"} {
				rule, found := reg.Lookup(endpoint)
				require.False(t, found, "nothing is registered for %s", endpoint)
				require.Nil(t, rule)
			}
		})
	}
}

// SetDefault(nil) removes the default instead of installing a nil rule.
func TestRegistry_SetDefaultNilRemovesDefault(t *testing.T) {
	t.Parallel()
	var reg endpointrule.Registry[nilRuleTestRule]
	reg.SetDefault(&nilRuleTestRule{name: "default"})

	reg.SetDefault(nil)

	rule, found := reg.Lookup("/anything")
	require.False(t, found)
	require.Nil(t, rule)
}
