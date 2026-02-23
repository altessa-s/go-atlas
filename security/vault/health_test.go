// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package vault_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/security/vault"

	vaultApi "github.com/hashicorp/vault/api"
)

func TestCheckHealth_NotRunning(t *testing.T) {
	client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	if err != nil {
		t.Fatalf("failed to create vault client: %v", err)
	}

	v, err := vault.New(t.Context(), vault.WithVaultClient(client))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	status := v.CheckHealth(t.Context())
	if status != health.StatusNotServing {
		t.Errorf("CheckHealth() = %v, want StatusNotServing", status)
	}
}
