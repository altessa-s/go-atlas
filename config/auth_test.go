// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuthMTLSAndScopeValidate(t *testing.T) {
	t.Parallel()
	// Only the new aggregate members are set; nil OIDC/OPA are skipped.
	mtls := DefaultMTLS()
	scope := ScopeRegistry{Rules: []ScopeRule{{Scope: "files:read", Keys: []string{"/files.v1.Files/Read"}}}}
	a := Auth{MTLS: &mtls, Scope: &scope}
	require.NoError(t, a.Validate())
}

func TestAuthIsEnabled(t *testing.T) {
	t.Parallel()

	var none Auth
	require.False(t, none.IsEnabled())

	mtls := DefaultMTLS()
	require.True(t, (&Auth{MTLS: &mtls}).IsEnabled())

	scope := ScopeRegistry{Rules: []ScopeRule{{Scope: "files:read", Keys: []string{"/files.v1.Files/Read"}}}}
	require.True(t, (&Auth{Scope: &scope}).IsEnabled())
}

func TestAuthEmptyScopeIsValidAndDisabled(t *testing.T) {
	t.Parallel()
	empty := DefaultScopeRegistry()
	a := Auth{Scope: &empty}
	require.NoError(t, a.Validate()) // empty registry must not fail validation
	require.False(t, a.IsEnabled())  // ...and counts as disabled
}
