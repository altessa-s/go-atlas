// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotencyit_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/idempotency"

	idemhttp "github.com/altessa-s/go-atlas/transport/http/server/middlewares/idempotency"
)

// newKeeper builds a Keeper over a fresh storage of b.
func newKeeper(tb testing.TB, b backend, opts ...idempotency.Option) *idempotency.Keeper {
	tb.Helper()
	opts = append([]idempotency.Option{idempotency.WithMaxLockDuration(idempotency.DefaultMaxLockDuration)}, opts...)
	return idempotency.New(b.open(tb, longTTL), opts...)
}

// TestKeeper_ReplaysTheCompletedResult is the request lifecycle: the first
// attempt runs, a duplicate during processing sees it in progress, and one
// after completion gets the stored result.
func TestKeeper_ReplaysTheCompletedResult(t *testing.T) {
	t.Parallel()
	forEachBackend(t, func(t *testing.T, b backend) {
		k := newKeeper(t, b)
		ctx := t.Context()

		acquired, held, err := k.AttemptLock(ctx, "payment-1")
		require.NoError(t, err)
		require.True(t, acquired)

		acquired, state, err := k.AttemptLock(ctx, "payment-1")
		require.NoError(t, err)
		require.False(t, acquired)
		require.Equal(t, idempotency.StatusInProgress, state.Status)

		require.NoError(t, k.Complete(ctx, "payment-1", "entity-42", held))

		acquired, state, err = k.AttemptLock(ctx, "payment-1")
		require.NoError(t, err)
		require.False(t, acquired)
		require.Equal(t, idempotency.StatusSuccess, state.Status)
		require.Equal(t, "entity-42", state.Data)

		// The denied state carries no token, so it cannot complete the key.
		require.ErrorIs(t, k.Complete(ctx, "payment-1", "forged", state), idempotency.ErrMissingLockState)
	})
}

// TestKeeper_ReleasedKeyCanBeRetried pins the failure path: a released key is
// free at once, and the released state cannot release the next holder.
func TestKeeper_ReleasedKeyCanBeRetried(t *testing.T) {
	t.Parallel()
	forEachBackend(t, func(t *testing.T, b backend) {
		k := newKeeper(t, b)
		ctx := t.Context()

		_, failed, err := k.AttemptLock(ctx, "payment-1")
		require.NoError(t, err)
		require.NoError(t, k.Release(ctx, "payment-1", failed))

		acquired, retry, err := k.AttemptLock(ctx, "payment-1")
		require.NoError(t, err)
		require.True(t, acquired)
		require.ErrorIs(t, k.Release(ctx, "payment-1", failed), idempotency.ErrLockStolen)
		require.NoError(t, k.Complete(ctx, "payment-1", "ok", retry))
	})
}

// TestKeeper_ReclaimsAnOrphanedLock pins orphan reclaim through Steal: past
// MaxLockDuration a new caller takes the lock over, the crashed holder can
// no longer complete, and of many callers racing for the reclaim only one
// wins.
func TestKeeper_ReclaimsAnOrphanedLock(t *testing.T) {
	t.Parallel()
	forEachBackend(t, func(t *testing.T, b backend) {
		// Long enough that the reclaim race below finishes well inside it, so
		// no reclaimer can take the winner's fresh lock for an orphan too;
		// 1s proved too short when the whole integration suite shares the
		// backends.
		const maxLock = 3 * time.Second
		k := newKeeper(t, b, idempotency.WithMaxLockDuration(maxLock))
		ctx := t.Context()

		_, orphan, err := k.AttemptLock(ctx, "job-1")
		require.NoError(t, err)
		acquired, _, err := k.AttemptLock(ctx, "job-1")
		require.NoError(t, err)
		require.False(t, acquired, "a fresh lock must not be reclaimed")

		// The orphan check reads the lock's own timestamp; nothing on the
		// server changes when it passes, so there is nothing to poll.
		time.Sleep(maxLock + 100*time.Millisecond)

		var (
			wins   atomic.Int32
			winner atomic.Pointer[idempotency.State]
			wg     sync.WaitGroup
		)
		errs := make([]error, contenders)
		start := make(chan struct{})
		for i := range contenders {
			wg.Go(func() {
				<-start
				acquired, state, err := k.AttemptLock(ctx, "job-1")
				errs[i] = err
				if acquired {
					wins.Add(1)
					winner.Store(state)
				}
			})
		}
		close(start)
		wg.Wait()

		for i, err := range errs {
			require.NoError(t, err, "reclaimer %d", i)
		}
		require.EqualValues(t, 1, wins.Load(), "exactly one caller must reclaim the orphan")
		require.ErrorIs(t, k.Complete(ctx, "job-1", "late", orphan), idempotency.ErrLockStolen)
		require.NoError(t, k.Complete(ctx, "job-1", "reclaimed", winner.Load()))

		_, state, err := k.AttemptLock(ctx, "job-1")
		require.NoError(t, err)
		require.Equal(t, "reclaimed", state.Data)
	})
}

// TestKeeper_RejectsAKeyReusedForAnotherRequest pins the fingerprint check
// against the record as stored: both in progress and completed.
func TestKeeper_RejectsAKeyReusedForAnotherRequest(t *testing.T) {
	t.Parallel()
	forEachBackend(t, func(t *testing.T, b backend) {
		k := newKeeper(t, b)
		ctx := t.Context()
		original := idempotency.AttemptLockOpts{Fingerprint: []byte("POST /orders {\"qty\":1}")}
		other := idempotency.AttemptLockOpts{Fingerprint: []byte("POST /orders {\"qty\":2}")}

		_, held, err := k.AttemptLockWithOpts(ctx, "order-1", original)
		require.NoError(t, err)
		_, _, err = k.AttemptLockWithOpts(ctx, "order-1", other)
		require.ErrorIs(t, err, idempotency.ErrFingerprintMismatch)

		require.NoError(t, k.Complete(ctx, "order-1", "done", held))
		_, _, err = k.AttemptLockWithOpts(ctx, "order-1", other)
		require.ErrorIs(t, err, idempotency.ErrFingerprintMismatch)
		_, state, err := k.AttemptLockWithOpts(ctx, "order-1", original)
		require.NoError(t, err)
		require.Equal(t, "done", state.Data)
	})
}

type tenantKey struct{}

// TestKeeper_NamespacesIsolateTenants pins WithKeyNamespace on a shared
// store: tenants using the same key each get their own lock and result, and
// so do (tenant, key) pairs whose ':'-joined forms are equal, and a caller
// without a tenant passing another tenant's joined or composed key.
func TestKeeper_NamespacesIsolateTenants(t *testing.T) {
	t.Parallel()
	type request struct{ tenant, key string }
	requests := []request{
		{"acme", "invoice-1"},
		{"globex", "invoice-1"},
		{"a:b", "c"},
		{"a", "b:c"},
		{"", "acme:invoice-1"},
		{"", idempotency.ComposeKey("namespace", "acme", "invoice-1")},
	}
	forEachBackend(t, func(t *testing.T, b backend) {
		k := newKeeper(t, b, idempotency.WithKeyNamespace(func(ctx context.Context) string {
			v, _ := ctx.Value(tenantKey{}).(string)
			return v
		}))
		ctxOf := func(r request) context.Context { return context.WithValue(t.Context(), tenantKey{}, r.tenant) }

		for i, r := range requests {
			acquired, held, err := k.AttemptLock(ctxOf(r), r.key)
			require.NoError(t, err)
			require.True(t, acquired, "request %d (%q, %q) must not share a lock", i, r.tenant, r.key)
			require.NoError(t, k.Complete(ctxOf(r), r.key, float64(i), held))
		}
		for i, r := range requests {
			_, state, err := k.AttemptLock(ctxOf(r), r.key)
			require.NoError(t, err)
			require.Equal(t, float64(i), state.Data, "request %d (%q, %q) must replay its own result", i, r.tenant, r.key)
		}
	})
}

// server runs the HTTP idempotency middleware over a Keeper on b in front of
// handler.
func server(t *testing.T, b backend, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(idemhttp.Middleware(newKeeper(t, b))(handler))
	t.Cleanup(srv.Close)
	return srv
}

// post sends body to srv with the idempotency key.
func post(t *testing.T, srv *httptest.Server, key, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/orders", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set(idemhttp.DefaultIdempotencyKeyHeader, key)
	resp, err := srv.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// TestMiddleware_ReplayIsRefused drives the middleware end to end: the
// handler runs once per key, a replay is refused with the success status,
// and the key reused with another body is refused as a mismatch.
func TestMiddleware_ReplayIsRefused(t *testing.T) {
	t.Parallel()
	forEachBackend(t, func(t *testing.T, b backend) {
		var calls atomic.Int32
		srv := server(t, b, func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			w.WriteHeader(http.StatusCreated)
		})
		key := uuid.NewString()

		require.Equal(t, http.StatusCreated, post(t, srv, key, `{"qty":1}`).StatusCode)

		replay := post(t, srv, key, `{"qty":1}`)
		require.Equal(t, http.StatusUnprocessableEntity, replay.StatusCode)
		require.Equal(t, "success", replay.Header.Get(idemhttp.DefaultIdempotencyKeyStatusHeader))

		mismatch := post(t, srv, key, `{"qty":2}`)
		require.Equal(t, http.StatusUnprocessableEntity, mismatch.StatusCode)
		require.Empty(t, mismatch.Header.Get(idemhttp.DefaultIdempotencyKeyStatusHeader), "a mismatch is not a replay")

		require.EqualValues(t, 1, calls.Load())
	})
}

// TestMiddleware_ConcurrentDuplicateIsRefused pins the in-progress branch: a
// duplicate arriving while the first request is still in its handler gets
// 409 and never reaches the handler.
func TestMiddleware_ConcurrentDuplicateIsRefused(t *testing.T) {
	t.Parallel()
	forEachBackend(t, func(t *testing.T, b backend) {
		var calls atomic.Int32
		entered, release := make(chan struct{}), make(chan struct{})
		srv := server(t, b, func(w http.ResponseWriter, _ *http.Request) {
			if calls.Add(1) == 1 {
				close(entered)
				<-release
			}
			w.WriteHeader(http.StatusCreated)
		})
		// Registered after the server, so it runs before the server's Close,
		// which would otherwise wait on the parked handler forever.
		unpark := sync.OnceFunc(func() { close(release) })
		t.Cleanup(unpark)
		key := uuid.NewString()

		var (
			wg       sync.WaitGroup
			first    *http.Response
			firstErr error
		)
		answered := make(chan struct{})
		wg.Go(func() {
			defer close(answered)
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/orders", strings.NewReader(`{}`))
			if err != nil {
				firstErr = err
				return
			}
			req.Header.Set(idemhttp.DefaultIdempotencyKeyHeader, key)
			first, firstErr = srv.Client().Do(req)
		})
		select {
		case <-entered:
		case <-answered:
			require.NoError(t, firstErr)
			_ = first.Body.Close()
			require.FailNow(t, "the first request was answered without reaching the handler", "status %d", first.StatusCode)
		}

		dup := post(t, srv, key, `{}`)
		require.Equal(t, http.StatusConflict, dup.StatusCode)
		require.Equal(t, "in_progress", dup.Header.Get(idemhttp.DefaultIdempotencyKeyStatusHeader))

		unpark()
		wg.Wait()
		require.NoError(t, firstErr)
		_ = first.Body.Close()
		require.Equal(t, http.StatusCreated, first.StatusCode)
		require.EqualValues(t, 1, calls.Load())
	})
}

// TestMiddleware_FailedRequestCanBeRetried pins the release path: a handler
// failure frees the key, so the client's retry runs the handler again.
func TestMiddleware_FailedRequestCanBeRetried(t *testing.T) {
	t.Parallel()
	forEachBackend(t, func(t *testing.T, b backend) {
		var calls atomic.Int32
		srv := server(t, b, func(w http.ResponseWriter, _ *http.Request) {
			if calls.Add(1) == 1 {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			w.WriteHeader(http.StatusCreated)
		})
		key := uuid.NewString()

		require.Equal(t, http.StatusBadGateway, post(t, srv, key, `{}`).StatusCode)
		require.Equal(t, http.StatusCreated, post(t, srv, key, `{}`).StatusCode)
		require.Equal(t, http.StatusUnprocessableEntity, post(t, srv, key, `{}`).StatusCode)
		require.EqualValues(t, 2, calls.Load())
	})
}
