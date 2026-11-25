// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package storagetest

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/store/mongo"
)

// MetadataOwnership checks that callers may mutate a stored input or a loaded result.
// nilResult is the expected result of storing nil. Each subtest uses its own key.
func MetadataOwnership(t *testing.T, s mongo.Storage, nilResult *mongo.CursorMetadata) {
	t.Helper()
	t.Run("Nil", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, s.Store(t.Context(), "nil", nil))
		got, err := s.Load(t.Context(), "nil")
		require.NoError(t, err)
		require.Equal(t, nilResult, got)
	})
	t.Run("Store", func(t *testing.T) {
		t.Parallel()
		input := &mongo.CursorMetadata{Subject: "owner"}
		require.NoError(t, s.Store(t.Context(), "input", input))
		input.Subject = "changed"
		got, err := s.Load(t.Context(), "input")
		require.NoError(t, err)
		require.Equal(t, "owner", got.Subject)
	})
	t.Run("Load", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, s.Store(t.Context(), "output", &mongo.CursorMetadata{Subject: "owner"}))
		got, err := s.Load(t.Context(), "output")
		require.NoError(t, err)
		other, err := s.Load(t.Context(), "output")
		require.NoError(t, err)

		// Independent readers can edit their results concurrently.
		var wg sync.WaitGroup
		wg.Go(func() { other.Subject = "other" })
		got.Subject = "changed"
		wg.Wait()
		require.Equal(t, "other", other.Subject)
		require.Equal(t, "changed", got.Subject)
		again, err := s.Load(t.Context(), "output")
		require.NoError(t, err)
		require.Equal(t, "owner", again.Subject)
	})
}
