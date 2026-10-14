// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/idempotency"
	"github.com/altessa-s/go-atlas/data/idempotency/storages/memory"

	middleware "github.com/altessa-s/go-atlas/transport/http/server/middlewares/idempotency"
)

func TestFailedRequestCannotReleaseNewOwner(t *testing.T) {
	t.Parallel()
	store := memory.New()
	keeper := idempotency.New(store)
	// key is the Keeper key the middleware builds; the Keeper stores it in the
	// empty namespace under storageKey.
	const key = "idk:v2:9:transport11:POST:/users36:550e8400-e29b-41d4-a716-446655440000"
	storageKey := idempotency.ComposeKey("namespace", "", key)
	handler := middleware.Middleware(keeper)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, current, _, err := store.AttemptLock(r.Context(), storageKey, nil)
		require.NoError(t, err)
		_, err = store.Steal(r.Context(), storageKey, current, append(current, ' '))
		require.NoError(t, err)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/users", nil)
	req.Header.Set("Idempotency-Key", "550e8400-e29b-41d4-a716-446655440000")
	handler.ServeHTTP(httptest.NewRecorder(), req)
	acquired, _, err := keeper.AttemptLock(t.Context(), key)
	require.NoError(t, err)
	require.False(t, acquired)
}
