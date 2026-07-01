// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/selfjwt"
)

func TestTokenPrincipal(t *testing.T) {
	t.Parallel()
	tok := selfjwt.Token{Subject: "alice", ID: "jti-1", Scopes: []string{"files:read"}}
	p := tok.Principal()
	require.Equal(t, "alice", p.Subject)
	require.Equal(t, []string{"files:read"}, p.Scopes)
	require.Empty(t, p.Tenant)
	require.Empty(t, p.Roles)
}
