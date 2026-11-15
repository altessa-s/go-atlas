// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiters_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/limiters"
)

func TestContextValues(t *testing.T) {
	t.Parallel()

	ctx := limiters.ContextWithClientIP(t.Context(), "192.0.2.1")
	ctx = limiters.ContextWithAuthToken(ctx, "secret")

	require.Equal(t, "192.0.2.1", limiters.ClientIPFromContext(ctx))
	require.Equal(t, "secret", limiters.AuthTokenFromContext(ctx))
	require.Empty(t, limiters.ClientIPFromContext(t.Context()))
	require.Empty(t, limiters.AuthTokenFromContext(t.Context()))
}
