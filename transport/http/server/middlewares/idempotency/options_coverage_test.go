// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/reliability/idempotency"
	"github.com/altessa-s/go-atlas/reliability/idempotency/storages"
	"github.com/altessa-s/go-atlas/reliability/idempotency/storages/memory"
	"github.com/altessa-s/go-atlas/transport/policy/fallback"

	middleware "github.com/altessa-s/go-atlas/transport/http/server/middlewares/idempotency"
)

const validKey = "550e8400-e29b-41d4-a716-446655440000"

// oneCaller binds every record to the same owner, as replay and entity ID
// exposure require.
var oneCaller = idempotency.WithOwner(func(context.Context) string { return "caller" })

// brokenStorage fails every lock attempt, standing in for an unreachable backend.
type brokenStorage struct{ storages.Storage }

var errBackend = errors.New("backend unreachable")

func (brokenStorage) AttemptLock(context.Context, string, []byte) (bool, []byte, []byte, error) {
	return false, nil, nil, errBackend
}

func (brokenStorage) AttemptLockWithTTL(context.Context, string, []byte, time.Duration) (bool, []byte, []byte, error) {
	return false, nil, nil, errBackend
}

func created() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/orders/42")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"42"}`))
	})
}

func post(t *testing.T, h http.Handler, target string, header map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, target, strings.NewReader(`{"sku":"a"}`))
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestName(t *testing.T) {
	t.Parallel()
	require.Equal(t, "idempotency", middleware.Name())
}

func TestStorageErrorFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		behavior fallback.Behavior
		wantCode int
	}{
		{name: "deny", behavior: fallback.Deny, wantCode: http.StatusServiceUnavailable},
		{name: "error", behavior: fallback.Error, wantCode: http.StatusInternalServerError},
		{name: "allow", behavior: fallback.Allow, wantCode: http.StatusCreated},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			keeper := idempotency.New(brokenStorage{Storage: memory.New()})
			h := middleware.Middleware(keeper, middleware.WithFallbackBehavior(tc.behavior))(created())
			rec := post(t, h, "/orders", map[string]string{"Idempotency-Key": validKey})
			require.Equal(t, tc.wantCode, rec.Code)
		})
	}
}

func TestCustomHeadersAndErrorHandler(t *testing.T) {
	t.Parallel()

	var scenarios []middleware.ErrorScenario
	keeper := idempotency.New(memory.New(), oneCaller)
	h := middleware.Middleware(keeper,
		middleware.WithIdempotencyKeyHeader("X-Request-Key"),
		middleware.WithIdempotencyKeyStatusHeader(new("X-Key-Status")),
		middleware.WithIdempotencyKeyEntityIDHeader(" X-Entity "),
		middleware.WithExposeEntityID(),
		middleware.WithEntityIDExtractor(func(*http.Request, string, []byte) string { return "42" }),
		middleware.WithEnforceMandatory(true),
		middleware.WithErrorHandler(func(w http.ResponseWriter, _ *http.Request, s middleware.ErrorScenario, _ *idempotency.State) {
			scenarios = append(scenarios, s)
			w.WriteHeader(http.StatusTeapot)
		}),
	)(created())

	rec := post(t, h, "/orders", nil)
	require.Equal(t, http.StatusTeapot, rec.Code, "missing key reaches the custom handler")

	rec = post(t, h, "/orders", map[string]string{"Idempotency-Key": validKey})
	require.Equal(t, http.StatusTeapot, rec.Code, "the default header name is no longer read")

	rec = post(t, h, "/orders", map[string]string{"X-Request-Key": validKey})
	require.Equal(t, http.StatusCreated, rec.Code)

	rec = post(t, h, "/orders", map[string]string{"X-Request-Key": validKey})
	require.Equal(t, http.StatusTeapot, rec.Code)
	require.Equal(t, "success", rec.Header().Get("X-Key-Status"))
	require.Equal(t, "42", rec.Header().Get("X-Entity"))

	require.Equal(t, []middleware.ErrorScenario{middleware.ErrorIDKMissing, middleware.ErrorIDKMissing, middleware.ErrorIDKAlreadyUsed}, scenarios)
}

func TestResponseReplayWithReplayHeaders(t *testing.T) {
	t.Parallel()

	keeper := idempotency.New(memory.New(), oneCaller)
	h := middleware.Middleware(keeper,
		middleware.WithResponseReplay(),
		middleware.WithReplayHeaders("location"),
		middleware.WithKeyQueryParam(new("idem")),
	)(created())

	first := post(t, h, "/orders?idem="+validKey, nil)
	require.Equal(t, http.StatusCreated, first.Code)

	again := post(t, h, "/orders?idem="+validKey, nil)
	require.Equal(t, http.StatusCreated, again.Code)
	require.Equal(t, `{"id":"42"}`, again.Body.String())
	require.Equal(t, "/orders/42", again.Header().Get("Location"))
}

func TestIgnoredPathsAndBlankOptions(t *testing.T) {
	t.Parallel()

	keeper := idempotency.New(memory.New())
	h := middleware.Middleware(keeper,
		middleware.WithIgnorePaths("/webhooks"),
		middleware.WithIgnorePatterns(regexp.MustCompile(`^/internal/`)),
		middleware.WithIdempotencyKeyHeader(""),
		middleware.WithIdempotencyKeyHeader((*string)(nil)),
		middleware.WithIdempotencyKeyStatusHeader(" "),
		middleware.WithIdempotencyKeyEntityIDHeader((*string)(nil)),
		middleware.WithKeyQueryParam(""),
		middleware.WithFingerprintTempDir((*string)(nil)),
		middleware.WithFingerprintTempDir(""),
		middleware.WithFingerprintTempDir(new(t.TempDir())),
		middleware.WithEnforceMandatory(true),
	)(created())

	for _, path := range []string{"/webhooks", "/internal/sync"} {
		for range 2 {
			rec := post(t, h, path, map[string]string{"Idempotency-Key": validKey})
			require.Equal(t, http.StatusCreated, rec.Code, path)
		}
	}

	rec := post(t, h, "/orders", map[string]string{"Idempotency-Key": validKey})
	require.Equal(t, http.StatusCreated, rec.Code, "blank options keep the default key header")
	rec = post(t, h, "/orders", map[string]string{"Idempotency-Key": validKey})
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Equal(t, "success", rec.Header().Get(middleware.DefaultIdempotencyKeyStatusHeader))
}
