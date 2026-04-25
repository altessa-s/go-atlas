// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	logger := slog.Default()

	b := New(nil).UseLogger(logger)

	assert.NotNil(t, b)
	assert.Equal(t, logger, b.Logger())
}

func TestNew_NoOptions(t *testing.T) {
	t.Parallel()

	b := New(nil)

	assert.NotNil(t, b)
	// New sets logger to DiscardHandler
	assert.NotNil(t, b.Logger())
}

func TestProvidersBuilder_CreateClientConfig_Nil(t *testing.T) {
	t.Parallel()

	b := New(nil)

	cfg, err := b.CreateClientConfig(nil)

	assert.NoError(t, err)
	assert.Nil(t, cfg)
}

func TestProvidersBuilder_CreateClientConfig_WithServerName(t *testing.T) {
	t.Parallel()

	b := New(nil)
	cfg := &config.TlsClient{
		ServerName: "example.com",
	}

	tlsConfig, err := b.CreateClientConfig(cfg)

	require.NoError(t, err)
	require.NotNil(t, tlsConfig)
	assert.Equal(t, "example.com", tlsConfig.ServerName)
	assert.False(t, tlsConfig.InsecureSkipVerify)
}

func TestProvidersBuilder_CreateClientConfig_SkipVerify_without_env(t *testing.T) {
	t.Setenv(config.EnvAllowInsecureTLS, "")

	b := New(nil)
	cfg := &config.TlsClient{
		ServerName: "example.com",
		SkipVerify: true,
	}
	cfg.Normalize() // env-guard resets SkipVerify

	tlsConfig, err := b.CreateClientConfig(cfg)

	require.NoError(t, err)
	require.NotNil(t, tlsConfig)
	assert.False(t, tlsConfig.InsecureSkipVerify, "InsecureSkipVerify must stay false without env guard")
}

func TestProvidersBuilder_CreateClientConfig_SkipVerify_with_env(t *testing.T) {
	t.Setenv(config.EnvAllowInsecureTLS, "true")

	b := New(nil)
	cfg := &config.TlsClient{
		ServerName: "example.com",
		SkipVerify: true,
	}
	cfg.Normalize() // env-guard permits SkipVerify

	tlsConfig, err := b.CreateClientConfig(cfg)

	require.NoError(t, err)
	require.NotNil(t, tlsConfig)
	assert.True(t, tlsConfig.InsecureSkipVerify, "InsecureSkipVerify must be true when env guard is set")
}

func TestProvidersBuilder_Build_NilConfig(t *testing.T) {
	t.Parallel()

	b := New(nil)

	providers, err := b.Build()

	require.NoError(t, err)
	require.NotNil(t, providers)
}

func TestProvidersBuilder_createFileProvider_InvalidFiles(t *testing.T) {
	t.Parallel()

	b := New(&config.TlsProvider{
		File: &config.TlsProviderFile{
			Certificate: "/nonexistent/cert.pem",
			PrivateKey:  "/nonexistent/key.pem",
		},
	})

	provider, err := b.createFileProvider()

	assert.Nil(t, provider)
	assert.Error(t, err)
}

func TestProvidersBuilder_createVaultProvider_NoClient(t *testing.T) {
	t.Parallel()

	b := New(&config.TlsProvider{
		Vault: &config.TlsProviderVault{
			CommonName: "example.com",
			Role:       "web",
		},
	})

	provider, err := b.createVaultProvider()

	assert.Nil(t, provider)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "vault client is required")
}

func TestProvidersBuilder_createLetsEncryptProvider_InvalidConfig(t *testing.T) {
	t.Parallel()

	b := New(&config.TlsProvider{
		LetsEncrypt: &config.TlsProviderLetsEncrypt{
			// Missing required fields
		},
	})

	provider, err := b.createLetsEncryptProvider()

	assert.Nil(t, provider)
	assert.Error(t, err)
}

func TestProvidersBuilder_fileProviderOpts(t *testing.T) {
	t.Parallel()

	t.Run("with logger", func(t *testing.T) {
		t.Parallel()

		logger := slog.Default()
		b := New(nil).UseLogger(logger)

		opts := b.fileProviderOpts()

		assert.Len(t, opts, 1)
	})

	t.Run("default builder", func(t *testing.T) {
		t.Parallel()

		b := New(nil)

		opts := b.fileProviderOpts()

		// Default builder has logger from New
		assert.Len(t, opts, 1)
	})
}

func TestProvidersBuilder_vaultProviderOpts(t *testing.T) {
	t.Parallel()

	t.Run("with logger and cache dir", func(t *testing.T) {
		t.Parallel()

		logger := slog.Default()
		b := New(nil).UseLogger(logger).UseCacheDir("/tmp/certs")

		opts := b.vaultProviderOpts()

		assert.Len(t, opts, 2)
	})

	t.Run("default builder", func(t *testing.T) {
		t.Parallel()

		b := New(nil)

		opts := b.vaultProviderOpts()

		// Default builder has logger from New
		assert.Len(t, opts, 1)
	})
}

func TestProvidersBuilder_letsEncryptProviderOpts(t *testing.T) {
	t.Parallel()

	t.Run("with logger and cache dir", func(t *testing.T) {
		t.Parallel()

		logger := slog.Default()
		b := New(nil).UseLogger(logger).UseCacheDir("/tmp/certs")

		opts := b.letsEncryptProviderOpts()

		assert.Len(t, opts, 2)
	})

	t.Run("default builder", func(t *testing.T) {
		t.Parallel()

		b := New(nil)

		opts := b.letsEncryptProviderOpts()

		// Default builder has logger from New
		assert.Len(t, opts, 1)
	})
}
