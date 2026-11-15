// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package vault_test

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/security/vault"

	vaultApi "github.com/hashicorp/vault/api"
)

// When the context given to RunRenewalWithContext ends after the first login,
// nothing renews a finite-lease token any more: the client must stop
// reporting healthy and say why, until renewal is started again.
func TestCheckHealth_RenewalContextEndedWithFiniteLease(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		method := &scriptedLogin{script: []func(*vaultApi.Client) (*vaultApi.Secret, error){issue(3600)}}
		v, err := vault.New(t.Context(), vault.WithVaultClient((&fakeVault{}).client(t)), vault.WithAuthMethod(method))
		require.NoError(t, err)
		t.Cleanup(func() { _ = v.Close() })

		startup, cancel := context.WithCancel(t.Context())
		require.NoError(t, v.RunRenewalWithContext(startup))
		require.Equal(t, health.StatusServing, v.CheckHealth(t.Context()))

		cancel()
		synctest.Wait()

		require.ErrorIs(t, v.RenewalErr(), vault.ErrRenewalCanceled)
		require.Equal(t, health.StatusNotServing, v.CheckHealth(t.Context()))

		require.NoError(t, v.RunRenewalWithContext(t.Context()))
		require.NoError(t, v.RenewalErr())
		require.Equal(t, health.StatusServing, v.CheckHealth(t.Context()), "a restart makes the client healthy again")

		require.NoError(t, v.StopRenewal())
		require.NoError(t, v.RenewalErr(), "an explicit stop is not an error")
		require.Equal(t, health.StatusNotServing, v.CheckHealth(t.Context()), "nothing renews the token after StopRenewal")
	})
}

// A token that never expires needs no renewal, so its run has already ended
// when the context does: the client stays healthy.
func TestCheckHealth_RenewalContextEndedWithNonExpiringToken(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		method := &scriptedLogin{script: []func(*vaultApi.Client) (*vaultApi.Secret, error){issue(0)}}
		v, err := vault.New(t.Context(), vault.WithVaultClient((&fakeVault{}).client(t)), vault.WithAuthMethod(method))
		require.NoError(t, err)
		t.Cleanup(func() { _ = v.Close() })

		startup, cancel := context.WithCancel(t.Context())
		require.NoError(t, v.RunRenewalWithContext(startup))
		synctest.Wait()

		cancel()
		synctest.Wait()

		require.NoError(t, v.RenewalErr())
		require.Equal(t, health.StatusServing, v.CheckHealth(t.Context()))
	})
}
