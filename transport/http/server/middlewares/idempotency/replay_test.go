// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/idempotency"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"

	internalidem "github.com/altessa-s/go-atlas/transport/internal/idempotency"
)

// created answers 201 with a numbered body, a Location, a JSON content type,
// a cookie and a request ID, counting its calls.
func created(calls *atomic.Int32) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Location", fmt.Sprintf("/orders/%d", n))
		w.Header().Set("Set-Cookie", "session=secret")
		w.Header().Set("X-Request-Id", fmt.Sprintf("req-%d", n))
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprintf(w, `{"id":%d,"echo":%q}`, n, body)
	})
}

// A repeat of a completed request receives the stored status, replay headers
// and body without running the handler again; per-request headers
// such as cookies are not replayed.
func TestResponseReplay(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	h := Middleware(newKeeper(), WithResponseReplay())(created(&calls))
	first := post(t, h, t.Context(), "/orders", `{"amount":10}`)
	require.Equal(t, http.StatusCreated, first.Code)

	second := post(t, h, t.Context(), "/orders", `{"amount":10}`)
	require.EqualValues(t, 1, calls.Load(), "the handler ran once")
	require.Equal(t, http.StatusCreated, second.Code)
	require.Equal(t, first.Body.String(), second.Body.String())
	require.Equal(t, "application/json", second.Header().Get("Content-Type"))
	require.Equal(t, "/orders/1", second.Header().Get("Location"))
	require.Equal(t, "success", second.Header().Get(DefaultIdempotencyKeyStatusHeader))
	require.Empty(t, second.Header().Get("Set-Cookie"), "cookies are never replayed")
	require.Empty(t, second.Header().Get("X-Request-Id"), "per-request headers are not replayed")

	require.Equal(t, http.StatusUnprocessableEntity, post(t, h, t.Context(), "/orders", `{"amount":99}`).Code,
		"a different request with the key gets no response")
}

func TestResponseReplayOffByDefault(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	h := Middleware(newKeeper())(created(&calls))
	require.Equal(t, http.StatusCreated, post(t, h, t.Context(), "/orders", "a").Code)
	require.Equal(t, http.StatusUnprocessableEntity, post(t, h, t.Context(), "/orders", "a").Code)
	require.EqualValues(t, 1, calls.Load())
}

// A body larger than the cap is not stored: the repeat is refused as already
// used. A cap above the capture limit is lowered to it.
func TestResponseReplaySizeCap(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	h := Middleware(newKeeper(), WithResponseReplay(), WithMaxReplaySize(8))(created(&calls))
	require.Equal(t, http.StatusCreated, post(t, h, t.Context(), "/orders", "a").Code)
	require.Equal(t, http.StatusUnprocessableEntity, post(t, h, t.Context(), "/orders", "a").Code)
	require.EqualValues(t, 1, calls.Load())

	big := New(newKeeper(), WithResponseReplay(), WithMaxReplaySize(1<<40))
	require.EqualValues(t, middlewares.MaxCaptureBodySize, big.opts.maxReplaySize)
	require.EqualValues(t, DefaultMaxReplaySize, New(newKeeper(), WithMaxReplaySize(0)).opts.maxReplaySize)
}

// A body beyond the capture limit is truncated by the capture, so it is never
// stored for replay, whatever the cap.
func TestResponseReplayNeverStoresATruncatedBody(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	huge := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(strings.Repeat("x", middlewares.MaxCaptureBodySize+1)))
	})
	m := New(newKeeper(), WithResponseReplay(), WithMaxReplaySize(middlewares.MaxCaptureBodySize))
	h := m.Handler(huge)
	require.Equal(t, http.StatusOK, post(t, h, t.Context(), "/orders", "a").Code)
	require.Equal(t, http.StatusUnprocessableEntity, post(t, h, t.Context(), "/orders", "a").Code)
	require.EqualValues(t, 1, calls.Load())
}

// A failed response stores nothing: the repeat runs the handler again.
func TestResponseReplayFailureNotStored(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	flaky := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusCreated)
	})
	h := Middleware(newKeeper(), WithResponseReplay())(flaky)
	require.Equal(t, http.StatusServiceUnavailable, post(t, h, t.Context(), "/orders", "a").Code)
	require.Equal(t, http.StatusCreated, post(t, h, t.Context(), "/orders", "a").Code)
	rec := post(t, h, t.Context(), "/orders", "a")
	require.Equal(t, http.StatusCreated, rec.Code, "the empty 201 is replayed")
	require.Empty(t, rec.Body.String())
	require.EqualValues(t, 2, calls.Load())
}

// Another owner's key is refused before anything of the record is written.
func TestResponseReplayOwnerChecked(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	k := newKeeper(idempotency.WithOwner(func(ctx context.Context) string { s, _ := ctx.Value(ownerCtxKey{}).(string); return s }))
	h := Middleware(k, WithResponseReplay())(created(&calls))
	alice := context.WithValue(t.Context(), ownerCtxKey{}, "alice")
	require.Equal(t, http.StatusCreated, post(t, h, alice, "/orders", "a").Code)
	rec := post(t, h, context.WithValue(t.Context(), ownerCtxKey{}, "bob"), "/orders", "a")
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.NotContains(t, rec.Body.String(), `"id"`)
	require.Equal(t, http.StatusCreated, post(t, h, alice, "/orders", "a").Code)
	require.EqualValues(t, 1, calls.Load())
}

// A record written before replay (a bare entity ID) has no response: the
// repeat is refused as already used, and the entity ID is still echoed when
// enabled.
func TestResponseReplayLegacyRecord(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	extract := WithEntityIDExtractor(func(*http.Request, string, []byte) string { return "entity-1" })
	k := newKeeper()
	require.Equal(t, http.StatusCreated, post(t, Middleware(k, extract)(created(&calls)), t.Context(), "/orders", "a").Code)

	rec := post(t, Middleware(k, extract, WithResponseReplay(), WithExposeEntityID())(created(&calls)), t.Context(), "/orders", "a")
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Equal(t, "entity-1", rec.Header().Get(DefaultIdempotencyKeyEntityIdHeader))
	require.EqualValues(t, 1, calls.Load())
}

// A record without a binding (written before records carried one) or bound
// to another operation is never replayed: the repeat is refused as already
// used.
func TestResponseReplayNeedsMatchingBinding(t *testing.T) {
	t.Parallel()

	for name, binding := range map[string]string{"no binding": "", "other operation": "PUT /orders"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			k := newKeeper()
			m := New(k, WithResponseReplay(), WithRequestFingerprint(false))
			key := m.buildKey(http.MethodPost, "/orders", httpTestKey)
			ok, held, err := k.AttemptLock(t.Context(), key)
			require.NoError(t, err)
			require.True(t, ok)
			require.NoError(t, k.Complete(t.Context(), key,
				internalidem.Record{Binding: binding, Status: http.StatusCreated, Body: []byte("stolen")}, held))

			var calls atomic.Int32
			rec := post(t, m.Handler(created(&calls)), t.Context(), "/orders", "a")
			require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
			require.NotContains(t, rec.Body.String(), "stolen")
			require.Zero(t, calls.Load())
		})
	}
}
