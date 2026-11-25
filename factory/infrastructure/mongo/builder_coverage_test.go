// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"crypto/tls"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"
	"github.com/altessa-s/go-atlas/store/mongo/kms"

	mongoconfig "github.com/altessa-s/go-atlas/config/mongo"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	kmslocal "github.com/altessa-s/go-atlas/store/mongo/kms/providers/local"
	converter "github.com/altessa-s/go-convert"
)

// fakeKMS is a local KMS provider with an optional TLS config.
type fakeKMS struct {
	tls *tls.Config
}

func (fakeKMS) Name() string { return "local" }
func (fakeKMS) Credentials() kms.Credentials {
	return kms.Credentials{"local": {"key": make([]byte, 96)}}
}
func (fakeKMS) Clear()                   {}
func (fakeKMS) MasterKey() kms.Key       { return nil }
func (f fakeKMS) TLSConfig() *tls.Config { return f.tls }

func baseConfig() *mongoconfig.Config {
	return &mongoconfig.Config{
		Hosts:          []string{"127.0.0.1:1"},
		Database:       "app",
		ConnectTimeout: time.Second,
	}
}

func TestBuild_NilConfig(t *testing.T) {
	t.Parallel()
	_, err := New(nil).Build(t.Context())
	require.ErrorIs(t, err, corefactory.ErrConfigRequired)
}

func TestBuild_RegistersHealthChecker(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		service  string
		wantName string
	}{
		{name: "default name", wantName: "mongo"},
		{name: "custom name", service: "orders-db", wantName: "orders-db"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			coord := health.New()
			t.Cleanup(coord.Close)

			m, err := New(baseConfig()).
				UseDefaultLogger().
				UseHealthCoordinator(coord).
				UseHealthServiceName(tc.service).
				UseMeter(metricstest.NewTestCollector()).
				UseConverterOptions(converter.WithValueMode(converter.IgnoreZeroValues)).
				Build(t.Context())
			require.NoError(t, err)
			require.Equal(t, "app", m.DatabaseName())
			require.Contains(t, slices.Collect(coord.ListServices()), tc.wantName)

			// Not connected yet: the checker reports the missing client.
			require.Equal(t, health.StatusNotServing, coord.CheckServiceHealth(t.Context(), tc.wantName))
		})
	}
}

func TestClientOptions_AppliesTLSConfig(t *testing.T) {
	t.Parallel()
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS13}
	opts, err := New(baseConfig()).UseTlsConfig(tlsCfg).ClientOptions()
	require.NoError(t, err)
	require.Same(t, tlsCfg, opts.TLSConfig)
}

func TestEncryption_RequiresKMSProvider(t *testing.T) {
	t.Parallel()
	cfg := baseConfig()
	cfg.Encryption = &mongoconfig.Encryption{Type: mongoconfig.EncryptionTypeAuto, VaultCollection: "__keyVault"}
	_, err := New(cfg).Build(t.Context())
	require.ErrorIs(t, err, corefactory.ErrDependencyRequired)
}

func TestEncryption_AutoConfiguresKeyVault(t *testing.T) {
	t.Parallel()
	vaultDB := "vault"
	local, err := kmslocal.New(kmslocal.WithMasterKey(strings.Repeat("k", kmslocal.RequiredMasterKeyLength)))
	require.NoError(t, err)
	t.Cleanup(local.Clear)
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	tests := []struct {
		name          string
		vaultDatabase *string
		provider      kms.MasterKeyProvider
		kmsTLS        *tls.Config
		wantNamespace string
	}{
		{name: "local provider without TLS capability", provider: local, wantNamespace: "app.__keyVault"},
		{name: "vault in app database", provider: fakeKMS{}, wantNamespace: "app.__keyVault"},
		{name: "separate vault database with tls", vaultDatabase: &vaultDB, provider: fakeKMS{tls: tlsConfig}, kmsTLS: tlsConfig, wantNamespace: "vault.__keyVault"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := baseConfig()
			cfg.Encryption = &mongoconfig.Encryption{
				Type: mongoconfig.EncryptionTypeAuto, VaultCollection: "__keyVault", VaultDatabase: tc.vaultDatabase,
			}
			b := New(cfg).UseKmsProvider(tc.provider)
			clientOpts, err := b.ClientOptions()
			require.NoError(t, err)
			opts, err := b.applyEncryption(clientOpts, nil)
			require.NoError(t, err)
			require.NotEmpty(t, opts)

			auto := clientOpts.AutoEncryptionOptions
			require.NotNil(t, auto)
			require.Equal(t, tc.wantNamespace, auto.KeyVaultNamespace)
			require.True(t, *auto.BypassAutoEncryption, "writes use explicit encryption")
			if tc.kmsTLS != nil {
				require.Same(t, tc.kmsTLS, auto.TLSConfig["local"])
			} else {
				require.Empty(t, auto.TLSConfig)
			}
		})
	}
}

func TestEncryption_ManualLeavesClientUnencrypted(t *testing.T) {
	t.Parallel()
	cfg := baseConfig()
	cfg.Encryption = &mongoconfig.Encryption{Type: mongoconfig.EncryptionTypeManual, VaultCollection: "__keyVault"}
	b := New(cfg).UseKmsProvider(fakeKMS{})
	clientOpts, err := b.ClientOptions()
	require.NoError(t, err)
	opts, err := b.applyEncryption(clientOpts, nil)
	require.NoError(t, err)
	require.NotEmpty(t, opts, "the wrapper still gets the KMS for explicit encryption")
	require.Nil(t, clientOpts.AutoEncryptionOptions)
}
