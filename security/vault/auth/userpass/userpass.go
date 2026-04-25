// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package userpass

import (
	"strings"

	"github.com/hashicorp/vault/api/auth/userpass"

	"github.com/altessa-s/go-atlas/security/vault/auth"

	vaultApi "github.com/hashicorp/vault/api"
)

// AuthMethod implements Vault username/password authentication.
type AuthMethod = auth.MountPathMethod

// Option configures AuthMethod.
type Option = auth.MethodOption[*AuthMethod]

// New creates a new userpass authentication method.
func New(username, password string, opt ...Option) *AuthMethod {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)

	return auth.NewMountPathMethodOptions("userpass", userpass.WithMountPath, func(opts ...userpass.LoginOption) (vaultApi.AuthMethod, error) {
		return userpass.NewUserpassAuth(username, &userpass.Password{FromString: password}, opts...)
	}, opt...)
}

// WithMountPath sets a custom mount path for the userpass auth method.
func WithMountPath[T interface{ string | *string }](path T) Option {
	return auth.WithMountPathOption[*AuthMethod](path)
}
