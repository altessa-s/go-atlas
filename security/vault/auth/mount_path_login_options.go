// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"context"

	vaultApi "github.com/hashicorp/vault/api"
)

// LoginWithMountPathOptions is a helper for auth methods that have:
// - a typed "WithMountPath" option, and
// - a constructor that accepts ...typedOptions.
//
// It appends mountPath into the option list in a consistent way and performs the login.
func LoginWithMountPathOptions[T any](
	ctx context.Context,
	client *vaultApi.Client,
	mountPath string,
	withMountPath func(string) T,
	newAuth func(opts ...T) (vaultApi.AuthMethod, error),
) (*vaultApi.Secret, error) {
	return LoginWithMountPath(ctx, client, mountPath, func(mountPath string) (vaultApi.AuthMethod, error) {
		opts := AppendMountPathOption([]T(nil), mountPath, withMountPath)
		return newAuth(opts...)
	})
}
