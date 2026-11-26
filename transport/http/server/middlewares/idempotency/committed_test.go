// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/text/strings/intern"
)

// serve runs h for one request, absorbing a panic the handler raises on purpose.
func serve(h http.Handler, req *http.Request) (rec *httptest.ResponseRecorder, panicked any) {
	rec = httptest.NewRecorder()
	defer func() { panicked = recover() }()
	h.ServeHTTP(rec, req)
	return rec, nil
}

// Two routes that share the decoded path of a request are two operations: a
// success of one is never replayed for the other.
func TestEscapedPathKeepsOperationsApart(t *testing.T) {
	t.Parallel()

	var single, grouped atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("POST /items/{id}", func(w http.ResponseWriter, _ *http.Request) {
		single.Add(1)
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("POST /items/{group}/{id}", func(w http.ResponseWriter, _ *http.Request) {
		grouped.Add(1)
		w.WriteHeader(http.StatusCreated)
	})
	h := Middleware(newOwnedKeeper(t), WithResponseReplay())(mux)

	require.Equal(t, http.StatusCreated, post(t, h, t.Context(), "/items/a%2Fb", "x").Code)
	require.Equal(t, http.StatusCreated, post(t, h, t.Context(), "/items/a/b", "x").Code)
	require.EqualValues(t, 1, single.Load())
	require.EqualValues(t, 1, grouped.Load(), "the second route ran")
}

// A flush commits the status the client sees: a later WriteHeader cannot turn
// the recorded outcome into a failure that releases the key.
func TestFlushCommitsStatusForKey(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	h := Middleware(newOwnedKeeper(t))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		require.NoError(t, http.NewResponseController(w).Flush())
		w.WriteHeader(http.StatusInternalServerError)
	}))

	post(t, h, t.Context(), "/orders", "a")
	post(t, h, t.Context(), "/orders", "a")
	require.EqualValues(t, 1, calls.Load(), "the key stayed taken after the flushed response")
}

// A handler that panicked after a 2xx status reached the client has run: a
// retry must not run it again, and a cut-off body is never replayed.
func TestPanicAfterCommittedSuccessKeepsKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		write func(w http.ResponseWriter)
	}{
		{name: "full body", write: func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":1}`)
		}},
		{name: "partial body", write: func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id`)
			_ = http.NewResponseController(w).Flush()
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var calls atomic.Int32
			h := Middleware(newOwnedKeeper(t), WithResponseReplay())(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				tc.write(w)
				panic(http.ErrAbortHandler)
			}))
			req := func() *http.Request {
				r := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader("a"))
				r.Header.Set(DefaultIdempotencyKeyHeader, httpTestKey)
				return r
			}

			_, p := serve(h, req())
			require.Equal(t, http.ErrAbortHandler, p)
			second, p := serve(h, req())
			require.Nil(t, p)
			require.EqualValues(t, 1, calls.Load(), "the handler did not run again")
			require.Equal(t, http.StatusUnprocessableEntity, second.Code, "a repeat is refused as already used")
			require.NotContains(t, second.Body.String(), `{"id`, "a truncated body is never replayed")
		})
	}
}

// A panic before any status reached the client still frees the key.
func TestPanicBeforeResponseReleasesKey(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	h := Middleware(newOwnedKeeper(t))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
		panic("boom")
	}))
	for range 2 {
		r := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader("a"))
		r.Header.Set(DefaultIdempotencyKeyHeader, httpTestKey)
		_, p := serve(h, r)
		require.Equal(t, "boom", p)
	}
	require.EqualValues(t, 2, calls.Load())
}

// Request-derived paths never enter the process-wide string interner.
func TestStorageKeyDoesNotIntern(t *testing.T) {
	// Not parallel: it measures the shared interner.
	m := New(newKeeper(t))
	before := intern.Default().Size()
	for i := range 2000 {
		_ = m.buildKey(http.MethodPost, "/orders/"+strconv.Itoa(i), httpTestKey)
	}
	require.Equal(t, before, intern.Default().Size())
}

// A handler that returns without writing sends an implicit 200: the key completes.
func TestImplicitOKCompletesKey(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	h := Middleware(newOwnedKeeper())(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))

	post(t, h, t.Context(), "/orders", "a")
	post(t, h, t.Context(), "/orders", "a")
	require.EqualValues(t, 1, calls.Load(), "the mutation ran once")
}

// failFlush is a writer whose flush fails after the status was committed.
type failFlush struct{ *httptest.ResponseRecorder }

func (failFlush) FlushError() error { return errors.New("broken pipe") }

// A failed flush may have committed the status: a later 500 does not release
// the key, and a response whose delivery failed is never replayed.
func TestFailedFlushCommitsAndIsNotReplayed(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	h := Middleware(newOwnedKeeper(), WithResponseReplay())(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, "created")
		_ = http.NewResponseController(w).Flush()
		w.WriteHeader(http.StatusInternalServerError)
	}))
	req := func() *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader("a"))
		r.Header.Set(DefaultIdempotencyKeyHeader, httpTestKey)
		return r
	}

	h.ServeHTTP(failFlush{httptest.NewRecorder()}, req())
	second := httptest.NewRecorder()
	h.ServeHTTP(failFlush{second}, req())
	require.EqualValues(t, 1, calls.Load(), "the key stayed taken")
	require.Equal(t, http.StatusUnprocessableEntity, second.Code, "the incompletely delivered response is not replayed")
}

// hijackable is a writer that hands its connection off.
type hijackable struct{ *httptest.ResponseRecorder }

func (hijackable) Hijack() (net.Conn, *bufio.ReadWriter, error) { return nil, nil, nil }

// A hijacked connection is a committed response: the key completes.
func TestHijackCompletesKey(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	h := Middleware(newOwnedKeeper())(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _, err := http.NewResponseController(w).Hijack()
		require.NoError(t, err)
	}))
	for range 2 {
		r := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader("a"))
		r.Header.Set(DefaultIdempotencyKeyHeader, httpTestKey)
		h.ServeHTTP(hijackable{httptest.NewRecorder()}, r)
	}
	require.EqualValues(t, 1, calls.Load())
}
