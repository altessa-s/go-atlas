// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package vault_test

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"
	"github.com/altessa-s/go-atlas/security/vault"
	"github.com/altessa-s/go-atlas/security/vault/auth"

	vaultApi "github.com/hashicorp/vault/api"
)

type recordingRegistrar struct {
	names    []string
	checkers []health.Checker
}

func (r *recordingRegistrar) RegisterService(name string, c health.Checker) {
	r.names = append(r.names, name)
	r.checkers = append(r.checkers, c)
}

// blockingMethod blocks until the context ends.
type blockingMethod struct{}

func (blockingMethod) Authenticate(ctx context.Context, _ *vaultApi.Client) (*vaultApi.Secret, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func (blockingMethod) Shutdown() error { return nil }
func (blockingMethod) Name() string    { return "blocking" }

// permanentFailMethod fails with a non-retryable error.
type permanentFailMethod struct{}

func (permanentFailMethod) Authenticate(context.Context, *vaultApi.Client) (*vaultApi.Secret, error) {
	return nil, auth.WrapAuthError("perm", auth.ErrInvalidCredentials)
}
func (permanentFailMethod) Shutdown() error { return nil }
func (permanentFailMethod) Name() string    { return "perm" }

var (
	_ auth.Method = blockingMethod{}
	_ auth.Method = permanentFailMethod{}
)

func TestNew_DefaultClientUsesTLSConfig(t *testing.T) {
	t.Parallel()
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS13, ServerName: "vault.example"}

	v, err := vault.New(t.Context(), vault.WithTLSConfig(tlsCfg), vault.WithLogger(slog.New(slog.DiscardHandler)))
	require.NoError(t, err)
	require.NotNil(t, v.RawClient())

	tr, ok := v.RawClient().CloneConfig().HttpClient.Transport.(*http.Transport)
	require.True(t, ok)
	require.Same(t, tlsCfg, tr.TLSClientConfig)
}

func TestNew_RegistersWithHealthCoordinator(t *testing.T) {
	t.Parallel()
	reg := &recordingRegistrar{}

	v, err := vault.New(t.Context(), vault.WithHealthCoordinator(reg))
	require.NoError(t, err)
	require.Equal(t, []string{"vault"}, reg.names)
	require.Same(t, health.Checker(v), reg.checkers[0])
}

func TestRunRenewal_TimeoutCountsError(t *testing.T) {
	t.Parallel()
	tc := metricstest.NewTestCollector()
	client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	require.NoError(t, err)
	v, err := vault.New(t.Context(),
		vault.WithVaultClient(client),
		vault.WithAuthMethod(blockingMethod{}),
		vault.WithAuthTimeout(20*time.Millisecond),
		vault.WithMeter(tc),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.StopRenewal() })

	err = v.RunRenewalWithContext(t.Context())
	require.ErrorIs(t, err, vault.ErrTimeout)
	require.InDelta(t, 1, metricstest.GetCounterValue(t, tc, "test_vault_renewal_attempts_total"), 0)
	require.InDelta(t, 1, metricstest.GetCounterValue(t, tc, "test_vault_renewal_errors_total"), 0)
}

func TestRunRenewal_PermanentErrorIsReturned(t *testing.T) {
	t.Parallel()
	client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	require.NoError(t, err)
	v, err := vault.New(t.Context(), vault.WithVaultClient(client), vault.WithAuthMethod(permanentFailMethod{}))
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.StopRenewal() })

	err = v.RunRenewalWithContext(t.Context())
	require.ErrorIs(t, err, auth.ErrInvalidCredentials)
}

func TestRunRenewal_CallerContextCanceled(t *testing.T) {
	t.Parallel()
	client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	require.NoError(t, err)
	v, err := vault.New(t.Context(), vault.WithVaultClient(client), vault.WithAuthMethod(blockingMethod{}))
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.StopRenewal() })

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	err = v.RunRenewalWithContext(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func healthVault(t *testing.T, body string, status int) *vault.Vault {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/sys/health", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	client, err := vaultApi.NewClient(&vaultApi.Config{Address: srv.URL, MaxRetries: 0})
	require.NoError(t, err)
	v, err := vault.New(t.Context(), vault.WithVaultClient(client), vault.WithAuthMethod(&gatedMethod{}))
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.StopRenewal() })
	require.NoError(t, v.RunRenewalWithContext(t.Context()))
	return v
}

func TestCheckHealth_Running(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		body   string
		status int
		want   health.ServingStatus
	}{
		{"healthy", `{"initialized":true,"sealed":false}`, http.StatusOK, health.StatusServing},
		{"sealed", `{"initialized":true,"sealed":true}`, http.StatusServiceUnavailable, health.StatusNotServing},
		{"uninitialized", `{"initialized":false,"sealed":false}`, http.StatusNotImplemented, health.StatusNotServing},
		{"malformed", `not json`, http.StatusOK, health.StatusNotServing},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			v := healthVault(t, tc.body, tc.status)
			require.Equal(t, tc.want, v.CheckHealth(t.Context()))
		})
	}
}

func TestCheckHealth_NotServingAfterStop(t *testing.T) {
	t.Parallel()
	v := healthVault(t, `{"initialized":true,"sealed":false}`, http.StatusOK)
	require.Equal(t, health.StatusServing, v.CheckHealth(t.Context()))
	require.NoError(t, v.StopRenewal())
	require.Equal(t, health.StatusNotServing, v.CheckHealth(t.Context()))
}
