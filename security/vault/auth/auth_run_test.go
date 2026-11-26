// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
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

type cancelAtLoginMethod struct {
	cancel context.CancelFunc
}

func (m cancelAtLoginMethod) Authenticate(_ context.Context, c *vaultApi.Client) (*vaultApi.Secret, error) {
	c.SetToken("t")
	m.cancel()
	return &vaultApi.Secret{
		Auth: &vaultApi.SecretAuth{
			ClientToken:   "t",
			Renewable:     true,
			LeaseDuration: 60,
		},
	}, nil
}
func (cancelAtLoginMethod) Shutdown() error { return nil }
func (cancelAtLoginMethod) Name() string    { return "cancel-at-login" }

func TestRun_CancelAtLoginDoesNotRaceOrPanic(t *testing.T) {
	t.Parallel()
	transport := testhelpers.RoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"auth":{"client_token":"t","renewable":true,"lease_duration":60}}`)),
			Request:    req,
		}, nil
	})
	client, err := vaultApi.NewClient(&vaultApi.Config{
		Address:    "http://vault.test",
		MaxRetries: 0,
		HttpClient: &http.Client{Transport: transport},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	a := auth.NewAuthenticator(client, cancelAtLoginMethod{cancel: cancel},
		auth.WithLogger(slog.New(slog.DiscardHandler)))
	require.NoError(t, a.Run(ctx))
}

type responseErrorMethod struct {
	err error
}

func (m responseErrorMethod) Authenticate(context.Context, *vaultApi.Client) (*vaultApi.Secret, error) {
	return nil, m.err
}
func (responseErrorMethod) Shutdown() error { return nil }
func (responseErrorMethod) Name() string    { return "userpass" }

func TestRun_LoginFailureLogsTypeAndStatusOnly(t *testing.T) {
	t.Parallel()
	client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	require.NoError(t, err)

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))

	rawErr := &vaultApi.ResponseError{
		HTTPMethod: http.MethodPut,
		URL:        "https://vault.internal.corp/v1/auth/userpass/login/alice-secret-user",
		StatusCode: http.StatusForbidden,
		Errors:     []string{"upstream-secret-marker"},
	}
	a := auth.NewAuthenticator(
		client,
		responseErrorMethod{err: auth.WrapAuthError("userpass", rawErr)},
		auth.WithLogger(logger),
	)

	err = a.Run(t.Context())
	require.Error(t, err)

	logged := logs.String()
	require.Contains(t, logged, "unable to authenticate")
	require.Contains(t, logged, "error_type=")
	require.Contains(t, logged, "403")
	require.NotContains(t, logged, "vault.internal.corp")
	require.NotContains(t, logged, "alice-secret-user")
	require.NotContains(t, logged, "upstream-secret-marker")
}
