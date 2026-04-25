// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tokenbucket_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/limiters/tokenbucket"
)

func TestExtractClientIp(t *testing.T) {
	t.Run("from context", func(t *testing.T) {
		ctx := tokenbucket.ContextWithClientIP(t.Context(), "10.0.0.1")
		got := tokenbucket.ExtractClientIp(ctx)
		require.Equal(t, "10.0.0.1", got)
	})

	t.Run("empty context", func(t *testing.T) {
		got := tokenbucket.ExtractClientIp(t.Context())
		require.Equal(t, "", got)
	})
}

func TestExtractAuthToken(t *testing.T) {
	t.Run("from context", func(t *testing.T) {
		ctx := tokenbucket.ContextWithAuthToken(t.Context(), "my-token")
		got := tokenbucket.ExtractAuthToken(ctx)
		require.Equal(t, "my-token", got)
	})

	t.Run("empty context", func(t *testing.T) {
		got := tokenbucket.ExtractAuthToken(t.Context())
		require.Equal(t, "", got)
	})
}
