// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/ext/kryptos"
	"github.com/altessa-s/go-atlas/observability/health"

	secretsconfig "github.com/altessa-s/go-atlas/config/secrets"
	kryptos "github.com/altessa-s/go-kryptos"
	memoryprovider "github.com/altessa-s/go-kryptos/providers/memory"
)

var errBuild = errors.New("build failed")

func memoryBuilder[T any](calls *[]string, name string, err error) factory.ProviderBuilder[T] {
	return func(context.Context, *secretsconfig.Config) (kryptos.Provider[T], error) {
		*calls = append(*calls, name)
		if err != nil {
			return nil, err
		}
		return memoryprovider.New[T](nil)
	}
}

func vaultConfig() *secretsconfig.Config {
	cfg := secretsconfig.Default()
	cfg.Provider = secretsconfig.ProviderVault
	return &cfg
}

func TestBuild_UnregisteredProviderFails(t *testing.T) {
	t.Parallel()

	for _, provider := range []secretsconfig.Provider{
		secretsconfig.ProviderVault, secretsconfig.ProviderGCP, secretsconfig.ProviderLockbox,
	} {
		t.Run(string(provider), func(t *testing.T) {
			t.Parallel()
			cfg := secretsconfig.Default()
			cfg.Provider = provider

			_, err := factory.New[any](&cfg).Build(t.Context())
			require.Error(t, err)
			require.NotErrorIs(t, err, factory.ErrProviderConfigMissing)
		})
	}
}

func TestUseProvider_NilBuilderFails(t *testing.T) {
	t.Parallel()

	_, err := factory.New[any](vaultConfig()).UseProvider(secretsconfig.ProviderVault, nil).Build(t.Context())
	require.Error(t, err)
}

func TestUseProvider_DispatchesByManagerType(t *testing.T) {
	t.Parallel()

	var calls []string

	m, err := factory.New[any](vaultConfig()).UseProvider(secretsconfig.ProviderVault, memoryBuilder[any](&calls, "any", nil)).Build(t.Context())
	require.NoError(t, err)
	require.NoError(t, m.Close(t.Context()))

	s, err := factory.New[string](vaultConfig()).UseProvider(secretsconfig.ProviderVault, memoryBuilder[string](&calls, "string", nil)).Build(t.Context())
	require.NoError(t, err)
	require.NoError(t, s.Close(t.Context()))

	require.Equal(t, []string{"any", "string"}, calls)
}

func TestUseProvider_BuilderErrorPropagates(t *testing.T) {
	t.Parallel()

	var calls []string
	_, err := factory.New[string](vaultConfig()).
		UseProvider(secretsconfig.ProviderVault, memoryBuilder[string](&calls, "string", errBuild)).
		Build(t.Context())
	require.ErrorIs(t, err, errBuild)
}

func TestBuild_RegisteredProviderRegistersHealth(t *testing.T) {
	t.Parallel()
	coord := health.New()
	t.Cleanup(coord.Close)

	var calls []string
	m, err := factory.New[any](vaultConfig()).
		UseDefaultLogger().
		UseLogger(slog.New(slog.DiscardHandler)).
		UseProvider(secretsconfig.ProviderVault, memoryBuilder[any](&calls, "any", nil)).
		UseHealthCoordinator(coord).
		Build(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = m.Close(context.WithoutCancel(t.Context())) })
	require.Contains(t, slices.Collect(coord.ListServices()), factory.HealthServiceName)
	require.Equal(t, health.StatusServing, coord.CheckServiceHealth(t.Context(), factory.HealthServiceName))
}
