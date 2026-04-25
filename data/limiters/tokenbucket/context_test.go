// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tokenbucket_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/limiters/tokenbucket"
)

func TestContextWithClientIP(t *testing.T) {
	t.Run("roundtrip", func(t *testing.T) {
		ctx := tokenbucket.ContextWithClientIP(t.Context(), "1.2.3.4")
		got := tokenbucket.ExtractClientIp(ctx)
		require.Equal(t, "1.2.3.4", got)
	})

	t.Run("empty context", func(t *testing.T) {
		got := tokenbucket.ExtractClientIp(t.Context())
		require.Equal(t, "", got)
	})

	t.Run("empty ip", func(t *testing.T) {
		ctx := tokenbucket.ContextWithClientIP(t.Context(), "")
		got := tokenbucket.ExtractClientIp(ctx)
		require.Equal(t, "", got)
	})
}

func TestContextWithAuthToken(t *testing.T) {
	t.Run("roundtrip", func(t *testing.T) {
		ctx := tokenbucket.ContextWithAuthToken(t.Context(), "my-token")
		got := tokenbucket.ExtractAuthToken(ctx)
		require.Equal(t, "my-token", got)
	})

	t.Run("empty context", func(t *testing.T) {
		got := tokenbucket.ExtractAuthToken(t.Context())
		require.Equal(t, "", got)
	})

	t.Run("empty token", func(t *testing.T) {
		ctx := tokenbucket.ContextWithAuthToken(t.Context(), "")
		got := tokenbucket.ExtractAuthToken(ctx)
		require.Equal(t, "", got)
	})
}
