// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/reliability/idempotency"
	"github.com/altessa-s/go-atlas/reliability/idempotency/storages/memory"

	middleware "github.com/altessa-s/go-atlas/transport/http/server/middlewares/idempotency"
)

// ctxKeeper fails Complete and Release under an ended context, as a storage
// reached over the network does.
type ctxKeeper struct{ *idempotency.Keeper }

func (k ctxKeeper) Complete(ctx context.Context, key string, data any, state *idempotency.State) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return k.Keeper.Complete(ctx, key, data, state)
}

func (k ctxKeeper) Release(ctx context.Context, key string, state *idempotency.State) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return k.Keeper.Release(ctx, key, state)
}

const (
	finalizeKey        = "550e8400-e29b-41d4-a716-446655440000"
	finalizeStorageKey = "idk:v2:9:transport11:POST:/users36:" + finalizeKey
)

func finalizeRequest(ctx context.Context) *http.Request {
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/users", nil)
	req.Header.Set("Idempotency-Key", finalizeKey)
	return req
}

func TestCanceledRequestStillFinalizesKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		status     int
		wantLocked bool
	}{
		{name: "success completes the key", status: http.StatusCreated, wantLocked: false},
		{name: "failure releases the key", status: http.StatusInternalServerError, wantLocked: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			keeper := idempotency.New(memory.New())
			ctx, cancel := context.WithCancel(t.Context())
			handler := middleware.Middleware(ctxKeeper{keeper})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				cancel() // the client disconnects while the handler runs
				w.WriteHeader(tc.status)
			}))

			handler.ServeHTTP(httptest.NewRecorder(), finalizeRequest(ctx))

			locked, state, err := keeper.AttemptLock(t.Context(), finalizeStorageKey)
			require.NoError(t, err)
			require.Equal(t, tc.wantLocked, locked)
			if !tc.wantLocked {
				require.Equal(t, idempotency.StatusSuccess, state.Status)
			}
		})
	}
}

func TestPanickingHandlerReleasesKey(t *testing.T) {
	t.Parallel()
	keeper := idempotency.New(memory.New())
	handler := middleware.Middleware(keeper)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))

	require.PanicsWithValue(t, "boom", func() {
		handler.ServeHTTP(httptest.NewRecorder(), finalizeRequest(t.Context()))
	})

	locked, _, err := keeper.AttemptLock(t.Context(), finalizeStorageKey)
	require.NoError(t, err)
	require.True(t, locked)
}

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
