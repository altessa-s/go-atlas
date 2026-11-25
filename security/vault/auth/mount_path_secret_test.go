// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/text/strings/secure"
	"github.com/altessa-s/go-atlas/security/vault/auth"

	vaultApi "github.com/hashicorp/vault/api"
)

// TestMountPathMethod_ShutdownWipesSecret pins the credential-zeroization contract:
// a secret attached via WithSecret must be wiped from memory when Shutdown runs,
// and Shutdown must be safe to call more than once.
func TestMountPathMethod_ShutdownWipesSecret(t *testing.T) {
	t.Parallel()

	s := secure.NewString("s3cr3t")
	m := auth.NewMountPathMethod(
		"test",
		func(context.Context, *vaultApi.Client, string) (*vaultApi.Secret, error) { return nil, nil },
		auth.WithSecret(s),
	)

	require.NoError(t, m.Shutdown())
	require.Empty(t, s.String(), "secret must be zeroed after Shutdown")
	require.NoError(t, m.Shutdown(), "Shutdown must be idempotent")
}

func TestMountPathMethod_ShutdownWithoutSecret(t *testing.T) {
	t.Parallel()
	calls := 0
	m := auth.NewMountPathMethod("custom", func(context.Context, *vaultApi.Client, string) (*vaultApi.Secret, error) {
		calls++
		return nil, nil
	})
	_, err := m.Authenticate(t.Context(), nil)
	require.NoError(t, err)
	require.NoError(t, m.Shutdown())
	_, err = m.Authenticate(t.Context(), nil)
	require.ErrorIs(t, err, auth.ErrMethodShutDown)
	var failure *auth.AuthError
	require.ErrorAs(t, err, &failure)
	require.Equal(t, m.Name(), failure.Method)
	require.Equal(t, 1, calls)
}

func TestMountPathMethod_ShutdownWaitsForCredentialRead(t *testing.T) {
	t.Parallel()
	secret := secure.NewString("credential")
	entered, resume := make(chan struct{}), make(chan struct{})
	m := auth.NewMountPathMethod("custom", func(context.Context, *vaultApi.Client, string) (*vaultApi.Secret, error) {
		close(entered)
		<-resume
		return &vaultApi.Secret{Auth: &vaultApi.SecretAuth{ClientToken: secret.String()}}, nil
	}, auth.WithSecret(secret))
	login := make(chan *vaultApi.Secret, 1)
	go func() {
		result, _ := m.Authenticate(t.Context(), nil)
		login <- result
	}()
	<-entered
	started, stopped := make(chan struct{}), make(chan error, 2)
	go func() { close(started); stopped <- m.Shutdown() }()
	<-started
	go func() { stopped <- m.Shutdown() }()
	select {
	case err := <-stopped:
		close(resume)
		<-login
		<-stopped
		require.FailNow(t, "Shutdown returned before the credential read", "error: %v", err)
	case <-time.After(50 * time.Millisecond):
		close(resume)
	}
	require.Equal(t, "credential", (<-login).Auth.ClientToken)
	require.NoError(t, <-stopped)
	require.NoError(t, <-stopped)
	require.Empty(t, secret.String())
}
