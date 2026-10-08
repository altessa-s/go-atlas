// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/idempotency"
	"github.com/altessa-s/go-atlas/data/idempotency/storages/memory"
)

const httpTestKey = "0b8a4f7e-6c1d-4e2a-9b3c-5d6e7f8a9b0c"

type ownerCtxKey struct{}

// echo answers 201 with the body it read.
var echo = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(body)
})

func post(t *testing.T, h http.Handler, ctx context.Context, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(body))
	req.Header.Set(DefaultIdempotencyKeyHeader, httpTestKey)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func newKeeper(opts ...idempotency.Option) *idempotency.Keeper {
	return idempotency.New(memory.New(), append([]idempotency.Option{idempotency.WithMaxLockDuration(time.Minute)}, opts...)...)
}

func TestRequestFingerprint(t *testing.T) {
	t.Parallel()

	h := Middleware(newKeeper())(echo)
	rec := post(t, h, t.Context(), "/orders?x=1", `{"amount":10}`)
	require.Equal(t, http.StatusCreated, rec.Code)
	require.Equal(t, `{"amount":10}`, rec.Body.String(), "the handler reads the whole body")

	require.Equal(t, http.StatusUnprocessableEntity, post(t, h, t.Context(), "/orders?x=1", `{"amount":10}`).Code, "a replay")
	rec = post(t, h, t.Context(), "/orders?x=1", `{"amount":99}`)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Contains(t, rec.Body.String(), "different request")
	require.Contains(t, post(t, h, t.Context(), "/orders?x=2", `{"amount":10}`).Body.String(), "different request", "the query counts")
}

func TestRequestFingerprintDisabled(t *testing.T) {
	t.Parallel()

	h := Middleware(newKeeper(), WithRequestFingerprint(false))(echo)
	require.Equal(t, http.StatusCreated, post(t, h, t.Context(), "/orders", "a").Code)
	rec := post(t, h, t.Context(), "/orders", "b")
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Contains(t, rec.Body.String(), "already been used")
}

func TestRequestFingerprintSpooledBody(t *testing.T) {
	t.Parallel()

	// A memory threshold below the body forces the temp-file spool.
	dir := t.TempDir()
	// echo, recording whether the spool file exists while the handler runs.
	var spilled atomic.Int32
	echo := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if entries, _ := os.ReadDir(dir); len(entries) > 0 {
			spilled.Add(1)
		}
		echo.ServeHTTP(w, r)
	})
	h := Middleware(newKeeper(), WithFingerprintMaxBody(64), WithFingerprintMemThreshold(8), WithFingerprintTempDir(dir))(echo)
	a, b := strings.Repeat("a", 40), strings.Repeat("a", 39)+"b"
	rec := post(t, h, t.Context(), "/orders", a)
	require.Equal(t, http.StatusCreated, rec.Code)
	require.Equal(t, a, rec.Body.String(), "the handler reads the spooled body intact")
	require.Equal(t, int32(1), spilled.Load(), "the body went through a temp file")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries, "the spool file is removed once the request is served")

	rec = post(t, h, t.Context(), "/orders", b)
	require.Contains(t, rec.Body.String(), "different request", "the whole body is fingerprinted")
}

func TestRequestFingerprintBodyTooLarge(t *testing.T) {
	t.Parallel()

	h := Middleware(newKeeper(), WithFingerprintMaxBody(8))(echo)
	require.Equal(t, http.StatusRequestEntityTooLarge, post(t, h, t.Context(), "/orders", "0123456789").Code)

	limited := Middleware(newKeeper())(echo)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/orders", strings.NewReader("0123456789"))
	req.Header.Set(DefaultIdempotencyKeyHeader, httpTestKey)
	rec := httptest.NewRecorder()
	req.Body = http.MaxBytesReader(rec, req.Body, 4) // as bodylimit would
	limited.ServeHTTP(rec, req)
	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestRequestFingerprintUnreadableBody(t *testing.T) {
	t.Parallel()

	h := Middleware(newKeeper())(echo)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/orders", io.MultiReader(strings.NewReader("ab"), failingReader{}))
	req.Header.Set(DefaultIdempotencyKeyHeader, httpTestKey)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	// Nothing was locked: the same key still works.
	require.Equal(t, http.StatusCreated, post(t, h, t.Context(), "/orders", "ab").Code)
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("connection reset") }

func TestFingerprintMaxBodyNormalized(t *testing.T) {
	t.Parallel()

	for _, v := range []int64{0, -1, math.MaxInt64} {
		h := Middleware(newKeeper(), WithFingerprintMaxBody(v))(echo)
		rec := post(t, h, t.Context(), "/orders", "body")
		require.Equal(t, http.StatusCreated, rec.Code, "limit %d", v)
		require.Equal(t, "body", rec.Body.String(), "limit %d", v)
	}
}

func TestOwnerForbidden(t *testing.T) {
	t.Parallel()

	k := newKeeper(idempotency.WithOwner(func(ctx context.Context) string { s, _ := ctx.Value(ownerCtxKey{}).(string); return s }))
	h := Middleware(k)(echo)
	alice := context.WithValue(t.Context(), ownerCtxKey{}, "alice")
	bob := context.WithValue(t.Context(), ownerCtxKey{}, "bob")
	require.Equal(t, http.StatusCreated, post(t, h, alice, "/orders", "a").Code)
	rec := post(t, h, bob, "/orders", "a")
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Empty(t, rec.Header().Get(DefaultIdempotencyKeyEntityIdHeader))
}

// trackingBody records whether it was closed.
type trackingBody struct {
	io.Reader
	closed atomic.Bool
}

func (b *trackingBody) Close() error {
	b.closed.Store(true)
	return nil
}

func TestOriginalBodyClosed(t *testing.T) {
	t.Parallel()

	for name, h := range map[string]http.Handler{
		"spooled":   Middleware(newKeeper())(echo),
		"too_large": Middleware(newKeeper(), WithFingerprintMaxBody(2))(echo),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			body := &trackingBody{Reader: strings.NewReader("payload")}
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/orders", body)
			req.Header.Set(DefaultIdempotencyKeyHeader, httpTestKey)
			h.ServeHTTP(httptest.NewRecorder(), req)
			require.True(t, body.closed.Load())
		})
	}
}
