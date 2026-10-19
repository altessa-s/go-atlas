// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	httpconfig "github.com/altessa-s/go-atlas/config/http"
	middlewareconfig "github.com/altessa-s/go-atlas/config/middleware"
	idempotencydata "github.com/altessa-s/go-atlas/data/idempotency"
)

// TestIdempotencyKeySourceFromConfig checks that the query parameter, key
// format and mandatory paths of the config reach the middleware.
func TestIdempotencyKeySourceFromConfig(t *testing.T) {
	t.Parallel()

	build := func(mutate func(*httpconfig.IdempotencyMiddleware)) *ServerBuilder {
		c := httpconfig.DefaultIdempotencyMiddleware()
		c.Enabled = true
		mutate(&c)
		return New(&httpconfig.Config{Middlewares: &httpconfig.Middlewares{Idempotency: &c}}).
			UseIdempotency(idempotencydata.New(testhelpers.NewMockIdempotencyStorage())).
			WithIdempotencyMiddleware()
	}
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusCreated) })
	send := func(h http.Handler, target string) int {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, target, http.NoBody))
		return rec.Code
	}

	b := build(func(c *httpconfig.IdempotencyMiddleware) {
		c.KeyQueryParam = "requestId"
		c.KeyFormat = middlewareconfig.IdempotencyKeyFormatOpaque
		c.MandatoryPaths = []string{"/orders"}
	})
	require.Empty(t, b.errs)
	require.Len(t, b.configMW, 1)
	h := b.configMW[0].Handler(ok)
	require.Equal(t, http.StatusBadRequest, send(h, "/orders"), "the listed path requires a key")
	require.Equal(t, http.StatusCreated, send(h, "/other"), "other paths keep enforceMandatory off")
	require.Equal(t, http.StatusCreated, send(h, "/orders?requestId=client-1"), "an opaque key from the query")
	require.Equal(t, http.StatusUnprocessableEntity, send(h, "/orders?requestId=client-1"))

	bad := build(func(c *httpconfig.IdempotencyMiddleware) {
		c.KeyFormat = middlewareconfig.IdempotencyKeyFormatPattern
		c.KeyPattern = "("
	})
	require.NotEmpty(t, bad.errs)
}

// TestIdempotencyResponseReplayFromConfig checks that responseReplay and
// replayHeaders reach the middleware.
func TestIdempotencyResponseReplayFromConfig(t *testing.T) {
	t.Parallel()

	c := httpconfig.DefaultIdempotencyMiddleware()
	c.Enabled = true
	c.ResponseReplay = true
	c.ReplayHeaders = []string{"Location"}
	b := New(&httpconfig.Config{Middlewares: &httpconfig.Middlewares{Idempotency: &c}}).
		UseIdempotency(idempotencydata.New(testhelpers.NewMockIdempotencyStorage())).
		WithIdempotencyMiddleware()
	require.Empty(t, b.errs)
	calls := 0
	h := b.configMW[0].Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Location", "/orders/1")
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("created"))
	}))
	send := func() *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/orders", http.NoBody)
		req.Header.Set("Idempotency-Key", "0b8a4f7e-6c1d-4e2a-9b3c-5d6e7f8a9b0c")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	require.Equal(t, http.StatusCreated, send().Code)
	rec := send()
	require.Equal(t, http.StatusCreated, rec.Code)
	require.Equal(t, "created", rec.Body.String())
	require.Equal(t, "/orders/1", rec.Header().Get("Location"))
	require.Empty(t, rec.Header().Get("Content-Type"), "only the configured headers are replayed")
	require.Equal(t, 1, calls)
}
