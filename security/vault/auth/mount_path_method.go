// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"context"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
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
	// secret holds credential material (e.g. a userpass password or an approle
	// secret ID) captured by loginFn. It is zeroed on Shutdown so the credential
	// does not linger in the heap after the method is retired.
	secret *corestrings.SecureString
}

// WithSecret attaches credential material to the method so it is zeroed on
// Shutdown. The same SecureString is read by the login function, so it must not
// be cleared until the method is retired.
func WithSecret(s *corestrings.SecureString) MethodOption[*MountPathMethod] {
	return func(m *MountPathMethod) { m.secret = s }
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

// Shutdown zeroes any attached credential material. It overrides the no-op
// BaseMethod.Shutdown so passwords and secret IDs are wiped from memory when the
// method is no longer needed.
func (m *MountPathMethod) Shutdown() error {
	if m.secret != nil {
		m.secret.Clear()
		m.secret = nil
	}
	return nil
}

var _ Method = (*MountPathMethod)(nil)
