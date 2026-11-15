// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secretsconfig_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-nomos/loader"
	"github.com/altessa-s/go-nomos/loader/backend/yaml3"

	secretsconfig "github.com/altessa-s/go-atlas/config/secrets"
)

func TestSecrets_EmptyListingThreshold(t *testing.T) {
	t.Parallel()
	cfg := secretsconfig.Default()
	require.Equal(t, 3, cfg.EmptyListingThreshold)
	require.False(t, cfg.AllowShallowClone)
	require.NoError(t, cfg.Validate())

	cfg.EmptyListingThreshold = 0 // manager default
	require.NoError(t, cfg.Validate())

	cfg.EmptyListingThreshold = -1
	require.Error(t, cfg.Validate())
}

// TestSecrets_LoadedMemorySection loads a memory provider section through the
// loader: the GCP and Lockbox sections stay nil when absent, so their required
// fields do not fail a configuration that does not use them.
func TestSecrets_LoadedMemorySection(t *testing.T) {
	t.Parallel()

	type root struct {
		Secrets *secretsconfig.Config `yaml:"secrets"`
	}
	path := filepath.Join(t.TempDir(), "secrets.yaml")
	content := "secrets:\n  provider: memory\n  memory:\n    values:\n      \"app:db_password\": s3cret\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	loaded, err := loader.New(&yaml3.Backend{}, loader.WithPath(path), loader.WithEnvPrefix("SECRETSCONFIGLOADTEST")).Load(&root{})
	require.NoError(t, err)
	cfg := loaded.(*root).Secrets //nolint:forcetypeassert // Load returns the pointer it was given
	require.Nil(t, cfg.GCP)
	require.Nil(t, cfg.Lockbox)
	require.Equal(t, "s3cret", cfg.Memory.Values["app:db_password"].Expose())
	require.NoError(t, cfg.Validate())
}
