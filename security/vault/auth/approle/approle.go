// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package approle

import (
	"strings"

	"github.com/hashicorp/vault/api/auth/approle"

	"github.com/altessa-s/go-atlas/security/vault/auth"

	vaultApi "github.com/hashicorp/vault/api"
)

// Option configures AuthMethod.
type Option = auth.MethodOption[*AuthMethod]

// WithMountPath sets a custom mount path for the AppRole auth method.
func WithMountPath[T interface{ string | *string }](path T) Option {
	return auth.WithMountPathOption[*AuthMethod](path)
}

// AuthMethod implements Vault AppRole authentication.
type AuthMethod = auth.MountPathMethod

func newLogin(roleID, secretID string) func(opts ...approle.LoginOption) (vaultApi.AuthMethod, error) {
	return func(opts ...approle.LoginOption) (vaultApi.AuthMethod, error) {
		return approle.NewAppRoleAuth(roleID, &approle.SecretID{FromString: secretID}, opts...)
	}
}

// New creates a new AppRole authentication method.
func New(roleID, secretID string, opt ...Option) *AuthMethod {
	roleID = strings.TrimSpace(roleID)
	secretID = strings.TrimSpace(secretID)

	return auth.NewMountPathMethodOptions("approle", approle.WithMountPath, newLogin(roleID, secretID), opt...)
}
