// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/security/vault/auth"

	vaultApi "github.com/hashicorp/vault/api"
)

// failingMethod always fails with a permanent authentication error.
type failingMethod struct{}

func (failingMethod) Authenticate(context.Context, *vaultApi.Client) (*vaultApi.Secret, error) {
	return nil, auth.WrapAuthError("failing", auth.ErrInvalidCredentials)
}
func (failingMethod) Shutdown() error { return nil }
func (failingMethod) Name() string    { return "failing" }

// TestRun_PermanentErrorDoesNotSignalReadiness: a run that ends on a permanent
// authentication error must report it and must not close FirstRenewCh, which
// would read as "token obtained".
func TestRun_PermanentErrorDoesNotSignalReadiness(t *testing.T) {
	t.Parallel()
	client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	require.NoError(t, err)
	a := auth.NewAuthenticator(client, failingMethod{})

	a.Run(t.Context()) // returns after the permanent error

	runErr, ok := <-a.ErrorsCh()
	require.True(t, ok)
	require.ErrorIs(t, runErr, auth.ErrInvalidCredentials)
	_, ok = <-a.ErrorsCh()
	require.False(t, ok, "ErrorsCh must be closed when Run returns")

	select {
	case <-a.FirstRenewCh():
		require.Fail(t, "FirstRenewCh closed although no token was obtained")
	default:
	}
}

// TestRun_CanceledDoesNotSignalReadiness: a canceled run ends without a token.
func TestRun_CanceledDoesNotSignalReadiness(t *testing.T) {
	t.Parallel()
	client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	require.NoError(t, err)
	a := auth.NewAuthenticator(client, failingMethod{})

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	a.Run(ctx)

	_, ok := <-a.ErrorsCh()
	require.False(t, ok)
	select {
	case <-a.FirstRenewCh():
		require.Fail(t, "FirstRenewCh closed although no token was obtained")
	default:
	}
}
