// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/security/vault/auth"
)

func TestBaseMethod_Name(t *testing.T) {
	base := auth.NewBaseMethod("approle")
	require.Equal(t, "approle", base.Name())
}

func TestBaseMethod_MountPath(t *testing.T) {
	base := auth.NewBaseMethod("approle")
	base.SetMountPath("  /auth/approle  ")
	require.Equal(t, "/auth/approle", base.MountPath())
}

func TestBaseMethod_Shutdown(t *testing.T) {
	base := auth.NewBaseMethod("approle")
	require.NoError(t, base.Shutdown())
}
