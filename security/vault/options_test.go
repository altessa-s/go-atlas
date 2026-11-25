// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package vault

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/security/vault/auth"
)

func TestWithAuthMethodPreservesPreviousOnNil(t *testing.T) {
	t.Parallel()
	method := auth.NewMountPathMethod("test", nil)
	opts := newOptions(WithAuthMethod(method), WithAuthMethod(nil), WithAuthMethod((*auth.MountPathMethod)(nil)))
	require.Same(t, method, opts.authMethod)
}
