// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package token_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/security/vault/auth"
	"github.com/altessa-s/go-atlas/security/vault/auth/token"

	vaultApi "github.com/hashicorp/vault/api"
)

// Authenticating after Shutdown must fail with a permanent error instead of
// dereferencing the zeroed token.
func TestAuthenticate_AfterShutdown(t *testing.T) {
	t.Parallel()
	client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	require.NoError(t, err)
	m := token.New("hvs.token")
	require.NoError(t, m.Shutdown())

	secret, err := m.Authenticate(t.Context(), client)

	require.Nil(t, secret)
	require.ErrorIs(t, err, auth.ErrMethodShutDown)
	require.True(t, auth.IsAuthenticationError(err), "the error must end the run, not be retried")
	require.Empty(t, client.Token())
}
