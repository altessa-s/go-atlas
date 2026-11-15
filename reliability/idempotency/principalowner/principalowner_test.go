// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package principalowner_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/principal"
	"github.com/altessa-s/go-atlas/reliability/idempotency/principalowner"
)

func TestPrincipalOwner(t *testing.T) {
	t.Parallel()

	label := func(p principal.Principal) string {
		return principalowner.PrincipalOwner(principal.NewContext(t.Context(), p))
	}

	require.Empty(t, principalowner.PrincipalOwner(t.Context()), "no principal, no owner")
	require.Empty(t, label(principal.Principal{Tenant: "acme"}), "a principal without a subject is no owner")
	require.NotEmpty(t, label(principal.Principal{Subject: "alice"}))
	require.Equal(t, label(principal.Principal{Subject: "alice", Tenant: "acme", Scopes: []string{"a"}}),
		label(principal.Principal{Subject: "alice", Tenant: "acme"}), "only tenant and subject make the label")
	require.NotEqual(t, label(principal.Principal{Subject: "alice", Tenant: "acme"}),
		label(principal.Principal{Subject: "alice", Tenant: "other"}), "the tenant is part of the label")
	require.NotEqual(t, label(principal.Principal{Subject: "b", Tenant: "a:"}),
		label(principal.Principal{Subject: ":b", Tenant: "a"}), "tenant and subject never run together")
}
