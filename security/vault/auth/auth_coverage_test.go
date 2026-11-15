// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth_test

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"
	"github.com/altessa-s/go-atlas/security/vault/auth"

	vaultApi "github.com/hashicorp/vault/api"
)

// scriptedMethod returns the results of its script in order; the last entry
// repeats once the script is exhausted.
type scriptedMethod struct {
	mu       sync.Mutex
	calls    int
	script   []func() (*vaultApi.Secret, error)
	shutdown atomic.Bool
}

func (m *scriptedMethod) Authenticate(context.Context, *vaultApi.Client) (*vaultApi.Secret, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	i := min(m.calls, len(m.script)-1)
	m.calls++
	return m.script[i]()
}

func (m *scriptedMethod) Calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func (m *scriptedMethod) Shutdown() error { m.shutdown.Store(true); return nil }

// IssuesToken marks the scripted logins as minting tokens, so a non-renewable
// finite-TTL token is refreshed by logging in again.
func (*scriptedMethod) IssuesToken() bool { return true }
func (m *scriptedMethod) Name() string    { return "scripted" }

func nonRenewable() (*vaultApi.Secret, error) {
	return &vaultApi.Secret{Auth: &vaultApi.SecretAuth{ClientToken: "t"}}, nil
}

func fastOpts(extra ...auth.Option) []auth.Option {
	return append([]auth.Option{
		auth.WithBackoffBase(time.Millisecond),
		auth.WithBackoffMax(5 * time.Millisecond),
		auth.WithAuthTimeout(time.Second),
		auth.WithLogger(slog.New(slog.DiscardHandler)),
	}, extra...)
}

func requireClosed(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		require.Fail(t, "channel was not closed")
	}
}

func TestRun_TransientErrorRetriesThenSignalsReadiness(t *testing.T) {
	t.Parallel()
	client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	require.NoError(t, err)
	tc := metricstest.NewTestCollector()
	m := &scriptedMethod{script: []func() (*vaultApi.Secret, error){
		func() (*vaultApi.Secret, error) { return nil, errors.New("connection reset") },
		nonRenewable,
	}}
	a := auth.NewAuthenticator(client, m, fastOpts(auth.WithMeter(tc))...)

	a.Run(t.Context())

	requireClosed(t, a.FirstRenewCh())
	_, ok := <-a.ErrorsCh()
	require.False(t, ok, "no error must be reported for a recovered transient failure")
	require.Equal(t, 2, m.Calls())
	require.True(t, m.shutdown.Load(), "the method must be shut down when Run returns")
	require.InDelta(t, 2, metricstest.GetCounterValue(t, tc, "test_vault_auth_auth_attempts_total"), 0)
	require.InDelta(t, 1, metricstest.GetCounterValue(t, tc, "test_vault_auth_auth_errors_total", "type", "transient"), 0)
}

func TestRun_PermanentErrorCountsMetric(t *testing.T) {
	t.Parallel()
	client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	require.NoError(t, err)
	tc := metricstest.NewTestCollector()
	a := auth.NewAuthenticator(client, failingMethod{}, fastOpts(auth.WithMeter(tc))...)

	a.Run(t.Context())

	require.ErrorIs(t, <-a.ErrorsCh(), auth.ErrInvalidCredentials)
	require.InDelta(t, 1, metricstest.GetCounterValue(t, tc, "test_vault_auth_auth_errors_total", "type", "permanent"), 0)
}

func renewServer(t *testing.T, h http.HandlerFunc) *vaultApi.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	client, err := vaultApi.NewClient(&vaultApi.Config{Address: srv.URL, MaxRetries: 0})
	require.NoError(t, err)
	client.SetToken("t")
	return client
}

func TestRun_RenewableTokenRenewsUntilCanceled(t *testing.T) {
	t.Parallel()
	var renewals atomic.Int32
	client := renewServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/auth/token/renew-self" {
			renewals.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"auth":{"client_token":"t","renewable":true,"lease_duration":3600}}`))
	})
	tc := metricstest.NewTestCollector()
	m := &scriptedMethod{script: []func() (*vaultApi.Secret, error){func() (*vaultApi.Secret, error) {
		return &vaultApi.Secret{Auth: &vaultApi.SecretAuth{ClientToken: "t", Renewable: true, LeaseDuration: 3600}}, nil
	}}}
	a := auth.NewAuthenticator(client, m, fastOpts(auth.WithMeter(tc))...)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		a.Run(ctx)
	}()

	requireClosed(t, a.FirstRenewCh())
	require.GreaterOrEqual(t, renewals.Load(), int32(1))
	cancel()
	requireClosed(t, done)

	_, ok := <-a.ErrorsCh()
	require.False(t, ok)
	require.Equal(t, 1, m.Calls())
	require.GreaterOrEqual(t, metricstest.GetCounterValue(t, tc, "test_vault_auth_token_renewals_total"), float64(1))
}

// When the lifetime watcher gives up (the lease runs out while renewals fail),
// the authenticator logs in again.
func TestRun_ExpiredLeaseTriggersReauthentication(t *testing.T) {
	t.Parallel()
	client := renewServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"errors":["internal server error"]}`))
	})
	m := &scriptedMethod{script: []func() (*vaultApi.Secret, error){
		func() (*vaultApi.Secret, error) {
			return &vaultApi.Secret{Auth: &vaultApi.SecretAuth{ClientToken: "t", Renewable: true, LeaseDuration: 1}}, nil
		},
		nonRenewable,
	}}
	a := auth.NewAuthenticator(client, m, fastOpts()...)

	done := make(chan struct{})
	go func() {
		defer close(done)
		a.Run(t.Context())
	}()

	requireClosed(t, done)
	require.Equal(t, 2, m.Calls())
	requireClosed(t, a.FirstRenewCh())
}

// fakeSDKAuth is a vaultApi.AuthMethod that returns a fixed secret.
type fakeSDKAuth struct {
	secret *vaultApi.Secret
	err    error
}

func (f fakeSDKAuth) Login(context.Context, *vaultApi.Client) (*vaultApi.Secret, error) {
	return f.secret, f.err
}

type mountOpt string

func TestNewMountPathMethodOptions_PassesMountPathAndSetsToken(t *testing.T) {
	t.Parallel()
	client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	require.NoError(t, err)

	var got []mountOpt
	m := auth.NewMountPathMethodOptions("custom",
		func(p string) mountOpt { return mountOpt(p) },
		func(opts ...mountOpt) (vaultApi.AuthMethod, error) {
			got = opts
			return fakeSDKAuth{secret: &vaultApi.Secret{Auth: &vaultApi.SecretAuth{ClientToken: "issued"}}}, nil
		},
		auth.WithMountPathOption[*auth.MountPathMethod]("  /Auth/Custom "),
	)
	require.Equal(t, "custom", m.Name())
	require.Equal(t, "/auth/custom", m.MountPath())

	secret, err := m.Authenticate(t.Context(), client)
	require.NoError(t, err)
	require.Equal(t, "issued", secret.Auth.ClientToken)
	require.Equal(t, []mountOpt{"/auth/custom"}, got)
	require.Equal(t, "issued", client.Token())
}

func TestNewMountPathMethodOptions_NoMountPathAddsNoOption(t *testing.T) {
	t.Parallel()
	client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	require.NoError(t, err)

	called := false
	m := auth.NewMountPathMethodOptions("custom",
		func(p string) mountOpt { return mountOpt(p) },
		func(opts ...mountOpt) (vaultApi.AuthMethod, error) {
			called = true
			require.Empty(t, opts)
			return fakeSDKAuth{secret: &vaultApi.Secret{Auth: &vaultApi.SecretAuth{ClientToken: "x"}}}, nil
		},
	)
	_, err = m.Authenticate(t.Context(), client)
	require.NoError(t, err)
	require.True(t, called)
}

func TestNewMountPathMethodOptions_BuildFailure(t *testing.T) {
	t.Parallel()
	client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	require.NoError(t, err)

	m := auth.NewMountPathMethodOptions("custom",
		func(p string) mountOpt { return mountOpt(p) },
		func(...mountOpt) (vaultApi.AuthMethod, error) { return nil, errors.New("bad input") },
	)
	_, err = m.Authenticate(t.Context(), client)
	require.ErrorIs(t, err, auth.ErrAuthInitializeFailed)
	authErr, ok := errors.AsType[*auth.AuthError](err)
	require.True(t, ok)
	require.Equal(t, "custom", authErr.Method)
}

func TestMountPathMethod_LoginErrorIsWrapped(t *testing.T) {
	t.Parallel()
	client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	require.NoError(t, err)

	m := auth.NewMountPathMethod("custom", func(context.Context, *vaultApi.Client, string) (*vaultApi.Secret, error) {
		return nil, &vaultApi.ResponseError{StatusCode: http.StatusUnauthorized, Errors: []string{"invalid secret id"}}
	})
	_, err = m.Authenticate(t.Context(), client)
	require.ErrorIs(t, err, auth.ErrInvalidCredentials)
	require.True(t, auth.IsAuthenticationError(err))
}

func TestWithMountPathOption(t *testing.T) {
	t.Parallel()
	nonEmpty := " /Auth/X "
	tests := []struct {
		name string
		opt  auth.MethodOption[*auth.BaseMethod]
		want string
	}{
		{"string lowercased and trimmed", auth.WithMountPathOption[*auth.BaseMethod]("  /Auth/AppRole "), "/auth/approle"},
		{"pointer", auth.WithMountPathOption[*auth.BaseMethod](&nonEmpty), "/auth/x"},
		{"nil pointer keeps previous", auth.WithMountPathOption[*auth.BaseMethod]((*string)(nil)), "/prev"},
		{"blank keeps previous", auth.WithMountPathOption[*auth.BaseMethod]("   "), "/prev"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := auth.NewBaseMethod("x")
			b.SetMountPath("/prev")
			auth.Apply(&b, tc.opt)
			require.Equal(t, tc.want, b.MountPath())
		})
	}
}

func TestAppendMountPathOption(t *testing.T) {
	t.Parallel()
	with := func(p string) string { return "mount=" + p }
	require.Equal(t, []string{"a"}, auth.AppendMountPathOption([]string{"a"}, "", with))
	require.Equal(t, []string{"a", "mount=/p"}, auth.AppendMountPathOption([]string{"a"}, "/p", with))
}

func TestBaseMethod_PerformLogin(t *testing.T) {
	t.Parallel()
	b := auth.NewBaseMethod("base")
	want := &vaultApi.Secret{RequestID: "r"}

	got, err := b.PerformLogin(t.Context(), nil, func(context.Context, *vaultApi.Client) (*vaultApi.Secret, error) {
		return want, nil
	})
	require.NoError(t, err)
	require.Same(t, want, got)

	cause := errors.New("boom")
	got, err = b.PerformLogin(t.Context(), nil, func(context.Context, *vaultApi.Client) (*vaultApi.Secret, error) {
		return nil, cause
	})
	require.Nil(t, got)
	require.ErrorIs(t, err, cause)
	authErr, ok := errors.AsType[*auth.AuthError](err)
	require.True(t, ok)
	require.Equal(t, "base", authErr.Method)
}

func TestRun_MethodWithoutAuthInfoReportsError(t *testing.T) {
	t.Parallel()
	for name, result := range map[string]func() (*vaultApi.Secret, error){
		"nil secret": func() (*vaultApi.Secret, error) { return nil, nil },
		"nil auth":   func() (*vaultApi.Secret, error) { return &vaultApi.Secret{}, nil },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
			require.NoError(t, err)
			tc := metricstest.NewTestCollector()
			m := &scriptedMethod{script: []func() (*vaultApi.Secret, error){result}}
			a := auth.NewAuthenticator(client, m, fastOpts(auth.WithMeter(tc))...)

			a.Run(t.Context())

			require.ErrorIs(t, <-a.ErrorsCh(), auth.ErrEmptyResponse)
			select {
			case <-a.FirstRenewCh():
				require.Fail(t, "readiness must not be signaled without a token")
			default:
			}
			require.Equal(t, 1, m.Calls())
			require.True(t, m.shutdown.Load())
			require.InDelta(t, 1, metricstest.GetCounterValue(t, tc, "test_vault_auth_auth_errors_total", "type", "permanent"), 0)
		})
	}
}
