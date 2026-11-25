// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/ext/tlsutils/vault"

	tlsconfig "github.com/altessa-s/go-atlas/config/tls"
	tlsfactory "github.com/altessa-s/go-atlas/factory/ext/tlsutils"
	tlsproviders "github.com/altessa-s/go-tlsutils/providers"
	vaultApi "github.com/hashicorp/vault/api"
)

func TestBuilder(t *testing.T) {
	t.Parallel()
	vc, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	require.NoError(t, err)

	cfg := &tlsconfig.Provider{Vault: &tlsconfig.ProviderVault{
		CommonName: "svc.example.com", Role: "web",
		SubjectAlternativeNames:   []string{"alt.example.com"},
		IPSubjectAlternativeNames: []string{"10.0.0.1"},
	}}
	p, err := factory.Builder(vc)(cfg, tlsfactory.ProviderEnv{Logger: slog.New(slog.DiscardHandler), CacheDir: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, p.Close(t.Context())) })
	require.Equal(t, tlsproviders.ProviderTypeVault, p.Type())
}

func TestBuilder_RequiresClient(t *testing.T) {
	t.Parallel()

	_, err := tlsfactory.New(&tlsconfig.Provider{Vault: &tlsconfig.ProviderVault{CommonName: "x", Role: "r"}}).
		UseProvider(tlsproviders.ProviderTypeVault, factory.Builder(nil)).
		Build(t.Context())
	require.ErrorIs(t, err, factory.ErrClientRequired)
}
