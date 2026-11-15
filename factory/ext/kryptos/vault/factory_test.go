// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/ext/kryptos/vault"

	secretsconfig "github.com/altessa-s/go-atlas/config/secrets"
	secretsfactory "github.com/altessa-s/go-atlas/factory/ext/kryptos"
	kryptos "github.com/altessa-s/go-kryptos"
	vaultApi "github.com/hashicorp/vault/api"
)

func vaultClient(t *testing.T) *vaultApi.Client {
	t.Helper()
	c, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	require.NoError(t, err)
	return c
}

func TestNewProvider(t *testing.T) {
	t.Parallel()

	_, err := factory.NewProvider[any](nil, nil)
	require.ErrorIs(t, err, factory.ErrClientRequired)

	p, err := factory.NewProvider[any](vaultClient(t), nil)
	require.NoError(t, err)
	require.Equal(t, "vault", p.Name())

	p, err = factory.NewProvider[any](vaultClient(t), &secretsconfig.Vault{MountPath: "secret", SecretPath: "app", CAS: true})
	require.NoError(t, err)
	require.Equal(t, "vault", p.Name())

	_, err = factory.NewProvider[any](vaultClient(t), &secretsconfig.Vault{MountPath: "kv", SecretPath: "../escape"})
	require.ErrorIs(t, err, kryptos.ErrInvalidOptions)
}

func TestBuilder(t *testing.T) {
	t.Parallel()
	cfg := secretsconfig.Default()
	cfg.Provider = secretsconfig.ProviderVault
	cfg.Vault = &secretsconfig.Vault{MountPath: "kv", SecretPath: "app"}

	m, err := secretsfactory.New(&cfg).
		UseProvider(secretsconfig.ProviderVault, factory.Builder(vaultClient(t))).
		Build(t.Context())
	require.NoError(t, err)
	require.NoError(t, m.Close(t.Context()))

	s, err := secretsfactory.New(&cfg).
		UseProvider(secretsconfig.ProviderVault, factory.Builder(vaultClient(t))).
		BuildString(t.Context())
	require.NoError(t, err)
	require.NoError(t, s.Close(t.Context()))
}

func TestBuilder_RequiresClient(t *testing.T) {
	t.Parallel()
	cfg := secretsconfig.Default()
	cfg.Provider = secretsconfig.ProviderVault

	_, err := secretsfactory.New(&cfg).UseProvider(secretsconfig.ProviderVault, factory.Builder(nil)).Build(t.Context())
	require.ErrorIs(t, err, factory.ErrClientRequired)
}
