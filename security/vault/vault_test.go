// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package vault_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/security/vault"
	"github.com/altessa-s/go-atlas/security/vault/auth"

	vaultApi "github.com/hashicorp/vault/api"
)

type mockMethod struct {
	name string
}

func (m *mockMethod) Authenticate(_ context.Context, _ *vaultApi.Client) (*vaultApi.Secret, error) {
	return nil, nil
}

func (m *mockMethod) Shutdown() error { return nil }

func (m *mockMethod) Name() string { return m.name }

var _ auth.Method = (*mockMethod)(nil)

func TestNew(t *testing.T) {
	client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	require.NoError(t, err)

	v, err := vault.New(t.Context(), vault.WithVaultClient(client))
	require.NoError(t, err)
	require.NotNil(t, v)
}

func TestNew_WithAuthMethod(t *testing.T) {
	client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	require.NoError(t, err)

	v, err := vault.New(t.Context(),
		vault.WithVaultClient(client),
		vault.WithAuthMethod(&mockMethod{name: "mock"}),
	)
	require.NoError(t, err)
	require.NotNil(t, v)
}

func TestRunRenewalWithContext_NilContext(t *testing.T) {
	client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	require.NoError(t, err)

	v, err := vault.New(t.Context(),
		vault.WithVaultClient(client),
		vault.WithAuthMethod(&mockMethod{name: "mock"}),
	)
	require.NoError(t, err)

	//nolint:staticcheck // intentionally passing nil context for test
	require.Error(t, v.RunRenewalWithContext(nil))
}

func TestRunRenewalWithContext_NoAuthMethod(t *testing.T) {
	client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	require.NoError(t, err)

	v, err := vault.New(t.Context(), vault.WithVaultClient(client))
	require.NoError(t, err)

	require.Error(t, v.RunRenewalWithContext(t.Context()))
}

func TestStopRenewal_NoActiveRenewal(t *testing.T) {
	client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	require.NoError(t, err)

	v, err := vault.New(t.Context(), vault.WithVaultClient(client))
	require.NoError(t, err)

	require.NoError(t, v.StopRenewal())
}

func TestRawClient(t *testing.T) {
	client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	require.NoError(t, err)

	v, err := vault.New(t.Context(), vault.WithVaultClient(client))
	require.NoError(t, err)

	require.Equal(t, client, v.RawClient())
}
