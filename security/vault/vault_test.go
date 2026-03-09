// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package vault_test

import (
	"context"
	"testing"

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
	if err != nil {
		t.Fatalf("failed to create vault client: %v", err)
	}

	v, err := vault.New(t.Context(), vault.WithVaultClient(client))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if v == nil {
		t.Fatal("New() returned nil")
	}
}

func TestNew_WithAuthMethod(t *testing.T) {
	client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	if err != nil {
		t.Fatalf("failed to create vault client: %v", err)
	}

	v, err := vault.New(t.Context(),
		vault.WithVaultClient(client),
		vault.WithAuthMethod(&mockMethod{name: "mock"}),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if v == nil {
		t.Fatal("New() returned nil")
	}
}

func TestRunRenewalWithContext_NilContext(t *testing.T) {
	client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	if err != nil {
		t.Fatalf("failed to create vault client: %v", err)
	}

	v, err := vault.New(t.Context(),
		vault.WithVaultClient(client),
		vault.WithAuthMethod(&mockMethod{name: "mock"}),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	//nolint:staticcheck // intentionally passing nil context for test
	if err := v.RunRenewalWithContext(nil); err == nil {
		t.Error("RunRenewalWithContext(nil) should return error")
	}
}

func TestRunRenewalWithContext_NoAuthMethod(t *testing.T) {
	client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	if err != nil {
		t.Fatalf("failed to create vault client: %v", err)
	}

	v, err := vault.New(t.Context(), vault.WithVaultClient(client))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := v.RunRenewalWithContext(t.Context()); err == nil {
		t.Error("RunRenewalWithContext without auth method should return error")
	}
}

func TestStopRenewal_NoActiveRenewal(t *testing.T) {
	client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	if err != nil {
		t.Fatalf("failed to create vault client: %v", err)
	}

	v, err := vault.New(t.Context(), vault.WithVaultClient(client))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := v.StopRenewal(); err != nil {
		t.Errorf("StopRenewal() error = %v, want nil", err)
	}
}

func TestRawClient(t *testing.T) {
	client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	if err != nil {
		t.Fatalf("failed to create vault client: %v", err)
	}

	v, err := vault.New(t.Context(), vault.WithVaultClient(client))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if got := v.RawClient(); got != client {
		t.Errorf("RawClient() returned different client")
	}
}
