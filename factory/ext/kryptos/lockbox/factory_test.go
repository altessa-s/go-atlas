// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-nomos/loader"
	"github.com/altessa-s/go-nomos/loader/backend/yaml3"

	"github.com/altessa-s/go-atlas/factory/ext/kryptos/lockbox"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	secretsconfig "github.com/altessa-s/go-atlas/config/secrets"
	secretsfactory "github.com/altessa-s/go-atlas/factory/ext/kryptos"
)

func TestNewProvider(t *testing.T) {
	t.Parallel()

	_, err := factory.NewProvider[any](t.Context(), nil)
	require.ErrorIs(t, err, secretsfactory.ErrProviderConfigMissing)

	_, err = factory.NewProvider[any](t.Context(), &secretsconfig.Lockbox{
		FolderID: "b1gfolder", KeyID: "ajekey", ServiceKeyID: "ajesa",
		PrivateKeyPath: filepath.Join(t.TempDir(), "missing.pem"),
	})
	require.Error(t, err)

	keyPath := filepath.Join(t.TempDir(), "key.pem")
	require.NoError(t, os.WriteFile(keyPath, testhelpers.GenerateRSAKeyPEM(t, 2048), 0o600))
	p, err := factory.NewProvider[any](t.Context(), &secretsconfig.Lockbox{
		FolderID: "b1gfolder", KeyID: "ajekey", ServiceKeyID: "ajesa",
		PrivateKeyPath:    keyPath,
		Labels:            map[string]string{"env": "prod"},
		IgnoreInvalidKeys: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = p.Close(context.WithoutCancel(t.Context())) })
	require.Equal(t, "lockbox", p.Name())
}

// Build and BuildString reject a lockbox provider whose section is absent
// with ErrProviderConfigMissing instead of dereferencing it.
func TestBuilder_MissingSection(t *testing.T) {
	t.Parallel()
	cfg := &secretsconfig.Config{Provider: secretsconfig.ProviderLockbox}

	_, err := secretsfactory.New(cfg).UseProvider(secretsconfig.ProviderLockbox, factory.Builder()).Build(t.Context())
	require.ErrorIs(t, err, secretsfactory.ErrProviderConfigMissing)
	_, err = secretsfactory.New(cfg).UseProvider(secretsconfig.ProviderLockbox, factory.Builder()).BuildString(t.Context())
	require.ErrorIs(t, err, secretsfactory.ErrProviderConfigMissing)
}

// A configuration loaded without a root Validate can select lockbox without
// its section; building from it fails with an error, not a panic.
func TestBuilder_LoadedConfigWithoutSection(t *testing.T) {
	t.Parallel()

	type root struct {
		Secrets *secretsconfig.Config `yaml:"secrets"`
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("secrets:\n  provider: lockbox\n"), 0o600))
	loaded, err := loader.New(&yaml3.Backend{}, loader.WithPath(path), loader.WithEnvPrefix("LOCKBOXSECRETSFACTORYLOADTEST")).Load(&root{})
	require.NoError(t, err)
	cfg := loaded.(*root).Secrets //nolint:forcetypeassert // Load returns the pointer it was given
	require.Nil(t, cfg.Lockbox)

	_, err = secretsfactory.New(cfg).UseProvider(secretsconfig.ProviderLockbox, factory.Builder()).BuildString(t.Context())
	require.ErrorIs(t, err, secretsfactory.ErrProviderConfigMissing)
}
