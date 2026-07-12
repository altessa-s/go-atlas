// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/security/vault/auth"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
	vaultApi "github.com/hashicorp/vault/api"
)

// TestMountPathMethod_ShutdownWipesSecret pins the credential-zeroization contract:
// a secret attached via WithSecret must be wiped from memory when Shutdown runs,
// and Shutdown must be safe to call more than once.
func TestMountPathMethod_ShutdownWipesSecret(t *testing.T) {
	t.Parallel()

	s := corestrings.NewSecureString("s3cr3t")
	m := auth.NewMountPathMethod(
		"test",
		func(context.Context, *vaultApi.Client, string) (*vaultApi.Secret, error) { return nil, nil },
		auth.WithSecret(s),
	)

	require.NoError(t, m.Shutdown())
	require.Empty(t, s.String(), "secret must be zeroed after Shutdown")
	require.NoError(t, m.Shutdown(), "Shutdown must be idempotent")
}
