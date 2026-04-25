// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"context"

	vaultApi "github.com/hashicorp/vault/api"
)

// NewMountPathMethodOptions creates a new MountPathMethod for auth methods that configure mount path
// via typed options and use Vault SDK auth constructors accepting ...typedOptions.
//
// This is a convenience wrapper around NewMountPathMethod + LoginWithMountPathOptions.
func NewMountPathMethodOptions[T any](
	name string,
	withMountPath func(string) T,
	newAuth func(opts ...T) (vaultApi.AuthMethod, error),
	opt ...MethodOption[*MountPathMethod],
) *MountPathMethod {
	return NewMountPathMethod(name, func(ctx context.Context, client *vaultApi.Client, mountPath string) (*vaultApi.Secret, error) {
		return LoginWithMountPathOptions(ctx, client, mountPath, withMountPath, newAuth)
	}, opt...)
}
