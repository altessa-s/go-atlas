// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth_test

import (
	"bytes"
	"context"
	"log/slog"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/security/vault/auth"

	vaultApi "github.com/hashicorp/vault/api"
)

func finiteNonRenewable() (*vaultApi.Secret, error) {
	return &vaultApi.Secret{Auth: &vaultApi.SecretAuth{ClientToken: "t", LeaseDuration: 1}}, nil
}

// A non-renewable token with a finite TTL must not end the run: the
// authenticator logs in again before the token expires and keeps the method's
// credentials alive until then.
func TestRun_NonRenewableFiniteTTLReauthenticates(t *testing.T) {
	t.Parallel()
	client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	require.NoError(t, err)
	reauthenticated := make(chan struct{})
	var shutdownBeforeReauth atomic.Bool
	m := &scriptedMethod{}
	m.script = []func() (*vaultApi.Secret, error){
		finiteNonRenewable,
		func() (*vaultApi.Secret, error) {
			shutdownBeforeReauth.Store(m.shutdown.Load())
			close(reauthenticated)
			return finiteNonRenewable()
		},
		finiteNonRenewable,
	}
	a := auth.NewAuthenticator(client, m, fastOpts()...)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()

	requireClosed(t, a.FirstRenewCh())
	requireClosed(t, reauthenticated)
	cancel()

	require.NoError(t, <-done, "cancellation is an expected exit")
	require.GreaterOrEqual(t, m.Calls(), 2)
	require.False(t, shutdownBeforeReauth.Load(), "the method must stay usable for re-authentication")
	require.True(t, m.shutdown.Load())
}

func TestRun_NeverExpiringTokenReturnsNil(t *testing.T) {
	t.Parallel()
	client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	require.NoError(t, err)
	m := &scriptedMethod{script: []func() (*vaultApi.Secret, error){nonRenewable}}
	a := auth.NewAuthenticator(client, m, fastOpts()...)

	require.NoError(t, a.Run(t.Context()))
	require.Equal(t, 1, m.Calls())
}

func TestRun_UnexpectedExitReturnsError(t *testing.T) {
	t.Parallel()
	client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	require.NoError(t, err)
	var logs bytes.Buffer
	a := auth.NewAuthenticator(client, failingMethod{},
		fastOpts(auth.WithLogger(slog.New(slog.NewTextHandler(&logs, nil))))...)

	require.ErrorIs(t, a.Run(t.Context()), auth.ErrInvalidCredentials)
	require.Contains(t, logs.String(), "vault authenticator stopped unexpectedly")
}

func TestMountPathMethod_IssuesToken(t *testing.T) {
	t.Parallel()
	var m auth.Method = auth.NewMountPathMethod("m", nil)
	issuer, ok := m.(auth.TokenIssuer)
	require.True(t, ok)
	require.True(t, issuer.IssuesToken())
}
