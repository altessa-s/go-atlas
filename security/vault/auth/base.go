// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"context"
	"strings"

	vaultApi "github.com/hashicorp/vault/api"
)

// BaseMethod provides common functionality for authentication methods.
// Embed this struct in concrete implementations to get common behavior.
type BaseMethod struct {
	// name is the authentication method name (e.g., "approle", "userpass")
	name string
	// mountPath is the custom mount path for the auth method
	mountPath string
}

// NewBaseMethod creates a new BaseMethod with the given name.
func NewBaseMethod(name string) BaseMethod {
	return BaseMethod{name: name}
}

// Name returns the authentication method name.
func (b *BaseMethod) Name() string {
	return b.name
}

// MountPath returns the configured mount path.
func (b *BaseMethod) MountPath() string {
	return b.mountPath
}

// SetMountPath sets the mount path, trimming whitespace.
func (b *BaseMethod) SetMountPath(path string) {
	b.mountPath = strings.TrimSpace(path)
}

// Shutdown performs cleanup. Default implementation does nothing.
// Override in concrete implementations if cleanup is needed.
func (b *BaseMethod) Shutdown() error {
	return nil
}

// LoginFunc is a function type that performs the actual Vault login.
// It encapsulates the Vault SDK-specific authentication logic.
type LoginFunc func(ctx context.Context, client *vaultApi.Client) (*vaultApi.Secret, error)

// PerformLogin executes the login function and wraps any errors with auth context.
// This provides consistent error handling across all auth methods.
func (b *BaseMethod) PerformLogin(ctx context.Context, client *vaultApi.Client, loginFn LoginFunc) (*vaultApi.Secret, error) {
	secret, err := loginFn(ctx, client)
	if err != nil {
		return nil, WrapAuthError(b.name, err)
	}
	return secret, nil
}
