// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package vault_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/security/vault"
	"github.com/altessa-s/go-atlas/security/vault/auth"

	vaultApi "github.com/hashicorp/vault/api"
)

// gatedMethod returns a non-renewable token. While gate is set, Authenticate
// signals entered and blocks until gate is closed (or ctx ends).
type gatedMethod struct {
	gate    atomic.Pointer[chan struct{}]
	entered chan struct{}
}

func (m *gatedMethod) Authenticate(ctx context.Context, _ *vaultApi.Client) (*vaultApi.Secret, error) {
	if g := m.gate.Load(); g != nil {
		m.entered <- struct{}{}
		select {
		case <-*g:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return &vaultApi.Secret{Auth: &vaultApi.SecretAuth{ClientToken: "t", Renewable: false}}, nil
}

func (m *gatedMethod) Shutdown() error { return nil }
func (m *gatedMethod) Name() string    { return "gated" }

var _ auth.Method = (*gatedMethod)(nil)

func newGatedVault(t *testing.T, m auth.Method) *vault.Vault {
	t.Helper()
	client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	require.NoError(t, err)
	v, err := vault.New(t.Context(), vault.WithVaultClient(client), vault.WithAuthMethod(m))
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.StopRenewal() })
	return v
}

// TestRunRenewal_RestartWaitsForNewToken: after a completed run, a restart
// must not report readiness from the previous run's channels, and finishing
// the new run must not panic on reused channels.
func TestRunRenewal_RestartWaitsForNewToken(t *testing.T) {
	t.Parallel()
	m := &gatedMethod{entered: make(chan struct{}, 1)}
	v := newGatedVault(t, m)

	require.NoError(t, v.RunRenewalWithContext(t.Context()))

	gate := make(chan struct{})
	m.gate.Store(&gate)
	result := make(chan error, 1)
	go func() { result <- v.RunRenewalWithContext(t.Context()) }()

	<-m.entered // the new run is authenticating and blocked
	select {
	case err := <-result:
		require.Failf(t, "restart returned before authentication completed", "err=%v", err)
	default:
	}

	close(gate)
	require.NoError(t, <-result)
	require.NoError(t, v.StopRenewal())
}

// TestRunRenewal_OverlappingRestarts runs renewals concurrently; each either
// gets its token or is superseded, and nothing panics or races.
func TestRunRenewal_OverlappingRestarts(t *testing.T) {
	t.Parallel()
	v := newGatedVault(t, &gatedMethod{entered: make(chan struct{}, 1)})

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if err := v.RunRenewalWithContext(t.Context()); err != nil && !errors.Is(err, vault.ErrRenewalStopped) {
				t.Errorf("unexpected renewal error: %v", err)
			}
		})
	}
	wg.Wait()
	require.NoError(t, v.StopRenewal())
}

// TestRunRenewal_StopDuringAuthentication: stopping a pending run reports
// ErrRenewalStopped (or the cancellation) instead of success.
func TestRunRenewal_StopDuringAuthentication(t *testing.T) {
	t.Parallel()
	m := &gatedMethod{entered: make(chan struct{}, 1)}
	gate := make(chan struct{})
	m.gate.Store(&gate)
	v := newGatedVault(t, m)

	result := make(chan error, 1)
	go func() { result <- v.RunRenewalWithContext(t.Context()) }()
	<-m.entered

	require.NoError(t, v.StopRenewal())
	require.ErrorIs(t, <-result, vault.ErrRenewalStopped)
}
