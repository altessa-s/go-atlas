// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package vault_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/security/vault"

	vaultApi "github.com/hashicorp/vault/api"
)

func TestCheckHealth_NotRunning(t *testing.T) {
	client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	require.NoError(t, err)

	v, err := vault.New(t.Context(), vault.WithVaultClient(client))
	require.NoError(t, err)

	status := v.CheckHealth(t.Context())
	require.Equal(t, health.StatusNotServing, status)
}
