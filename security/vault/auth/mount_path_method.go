// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"context"

	vaultApi "github.com/hashicorp/vault/api"
)

// MountPathLoginFunc is a Vault login function that may use a mount path.
// It is intended for thin method adapters (approle/userpass) to supply Vault SDK specifics.
type MountPathLoginFunc func(ctx context.Context, client *vaultApi.Client, mountPath string) (*vaultApi.Secret, error)

// MountPathMethod is a reusable auth method implementation that delegates Vault SDK specifics
// to a MountPathLoginFunc, while keeping common behavior in BaseMethod.
type MountPathMethod struct {
	BaseMethod
	loginFn MountPathLoginFunc
}

// NewMountPathMethod creates a new MountPathMethod with the given name and login function.
func NewMountPathMethod(name string, loginFn MountPathLoginFunc, opt ...MethodOption[*MountPathMethod]) *MountPathMethod {
	m := &MountPathMethod{
		BaseMethod: NewBaseMethod(name),
		loginFn:    loginFn,
	}
	Apply(m, opt...)
	return m
}

// Authenticate performs login using the provided Vault client.
func (m *MountPathMethod) Authenticate(ctx context.Context, client *vaultApi.Client) (*vaultApi.Secret, error) {
	return m.PerformLogin(ctx, client, func(ctx context.Context, client *vaultApi.Client) (*vaultApi.Secret, error) {
		return m.loginFn(ctx, client, m.MountPath())
	})
}

var _ Method = (*MountPathMethod)(nil)
