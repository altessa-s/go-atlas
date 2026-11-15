// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tokenbucket_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/limiters"
)

func TestExtractClientIp(t *testing.T) {
	t.Run("from context", func(t *testing.T) {
		ctx := limiters.ContextWithClientIP(t.Context(), "10.0.0.1")
		got := limiters.ClientIPFromContext(ctx)
		require.Equal(t, "10.0.0.1", got)
	})

	t.Run("empty context", func(t *testing.T) {
		got := limiters.ClientIPFromContext(t.Context())
		require.Equal(t, "", got)
	})
}

func TestExtractAuthToken(t *testing.T) {
	t.Run("from context", func(t *testing.T) {
		ctx := limiters.ContextWithAuthToken(t.Context(), "my-token")
		got := limiters.AuthTokenFromContext(ctx)
		require.Equal(t, "my-token", got)
	})

	t.Run("empty context", func(t *testing.T) {
		got := limiters.AuthTokenFromContext(t.Context())
		require.Equal(t, "", got)
	})
}
