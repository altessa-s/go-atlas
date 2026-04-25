// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

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

func TestNewAuthenticator(t *testing.T) {
	client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	require.NoError(t, err)

	a := auth.NewAuthenticator(client, &mockMethod{name: "mock"})
	require.NotNil(t, a)
}

func TestAuthenticator_ErrorsCh(t *testing.T) {
	client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	require.NoError(t, err)

	a := auth.NewAuthenticator(client, &mockMethod{name: "mock"})
	require.NotNil(t, a.ErrorsCh())
}

func TestAuthenticator_FirstRenewCh(t *testing.T) {
	client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	require.NoError(t, err)

	a := auth.NewAuthenticator(client, &mockMethod{name: "mock"})
	require.NotNil(t, a.FirstRenewCh())
}
