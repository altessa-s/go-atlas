// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tokenbucket_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/limiters"
)

func TestContextWithClientIP(t *testing.T) {
	t.Parallel()
	t.Run("roundtrip", func(t *testing.T) {
		ctx := limiters.ContextWithClientIP(t.Context(), "1.2.3.4")
		got := limiters.ClientIPFromContext(ctx)
		require.Equal(t, "1.2.3.4", got)
	})

	t.Run("empty context", func(t *testing.T) {
		got := limiters.ClientIPFromContext(t.Context())
		require.Equal(t, "", got)
	})

	t.Run("empty ip", func(t *testing.T) {
		ctx := limiters.ContextWithClientIP(t.Context(), "")
		got := limiters.ClientIPFromContext(ctx)
		require.Equal(t, "", got)
	})
}

func TestContextWithAuthToken(t *testing.T) {
	t.Parallel()
	t.Run("roundtrip", func(t *testing.T) {
		ctx := limiters.ContextWithAuthToken(t.Context(), "my-token")
		got := limiters.AuthTokenFromContext(ctx)
		require.Equal(t, "my-token", got)
	})

	t.Run("empty context", func(t *testing.T) {
		got := limiters.AuthTokenFromContext(t.Context())
		require.Equal(t, "", got)
	})

	t.Run("empty token", func(t *testing.T) {
		ctx := limiters.ContextWithAuthToken(t.Context(), "")
		got := limiters.AuthTokenFromContext(ctx)
		require.Equal(t, "", got)
	})
}
