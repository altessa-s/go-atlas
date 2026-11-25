// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	secretsconfig "github.com/altessa-s/go-atlas/config/secrets"
	factory "github.com/altessa-s/go-atlas/factory/ext/kryptos"
	kryptos "github.com/altessa-s/go-kryptos"
	memoryprovider "github.com/altessa-s/go-kryptos/providers/memory"
)

func TestBuildTypedProvider(t *testing.T) {
	t.Parallel()
	cfg := secretsconfig.Default()
	cfg.Provider = secretsconfig.ProviderVault
	mgr, err := factory.New[int](&cfg).UseProvider(secretsconfig.ProviderVault,
		func(context.Context, *secretsconfig.Config) (kryptos.Provider[int], error) {
			return memoryprovider.New(map[string]int{"app:count": 42})
		}).Build(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, mgr.Close(context.WithoutCancel(t.Context()))) })
	value, err := mgr.Get(t.Context(), "app:count")
	require.NoError(t, err)
	t.Cleanup(value.Clear)
	require.Equal(t, 42, value.Value)
}
