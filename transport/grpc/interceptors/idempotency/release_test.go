// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/idempotency"
	"github.com/altessa-s/go-atlas/data/idempotency/storages/memory"
)

// Directly exercise the private per-request driver after another caller steals its lock.
func TestPostCallCannotReleaseNewOwner(t *testing.T) {
	t.Parallel()
	store := memory.New()
	keeper := idempotency.New(store)
	acquired, old, err := keeper.AttemptLock(t.Context(), "key")
	require.NoError(t, err)
	require.True(t, acquired)
	_, current, _, err := store.AttemptLock(t.Context(), "key", nil)
	require.NoError(t, err)
	_, err = store.Steal(t.Context(), "key", current, append(current, ' '))
	require.NoError(t, err)
	ri := &requestInterceptor{interceptor: &interceptor{i: keeper}, lockedKey: "key", lockedState: old}
	boom := errors.New("handler failed")
	require.ErrorIs(t, ri.PostCall(t.Context(), nil, boom), boom)
	acquired, _, err = keeper.AttemptLock(t.Context(), "key")
	require.NoError(t, err)
	require.False(t, acquired)
}
