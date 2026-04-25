// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"context"

	vaultApi "github.com/hashicorp/vault/api"
)

// LoginWithMountPath builds a Vault SDK auth method using mountPath and performs client.Auth().Login.
// It centralizes the common "build -> validate -> login" flow for mountable auth methods.
func LoginWithMountPath(
	ctx context.Context,
	client *vaultApi.Client,
	mountPath string,
	build func(mountPath string) (vaultApi.AuthMethod, error),
) (*vaultApi.Secret, error) {
	authMethod, err := build(mountPath)
	if err != nil {
		return nil, ErrAuthInitializeFailed
	}
	return client.Auth().Login(ctx, authMethod)
}
