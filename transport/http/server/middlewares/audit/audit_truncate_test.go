// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/audit"

	audithttp "github.com/altessa-s/go-atlas/transport/http/server/middlewares/audit"
)

// auditedEvent runs one request through the middleware and returns the event
// it emitted.
func auditedEvent(t *testing.T, req *http.Request, opts ...audithttp.Option) *audit.Event {
	t.Helper()
	emitter := &fakeEmitter{}
	handler := audithttp.Middleware(emitter, opts...)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "an oversized value is truncated, the request is not rejected")
	require.NotNil(t, emitter.event)
	return emitter.event
}

// The URL path and the User-Agent are client-controlled; without a cap one
// request can write megabytes into the audit store.
func TestMiddleware_CapsPathAndUserAgent(t *testing.T) {
	t.Parallel()

	t.Run("defaults", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/"+strings.Repeat("a", 3*audithttp.DefaultMaxPathLength), nil)
		req.Header.Set("User-Agent", strings.Repeat("u", 3*audithttp.DefaultMaxUserAgentLength))
		event := auditedEvent(t, req)

		require.Len(t, event.Resource.Path, audithttp.DefaultMaxPathLength)
		require.True(t, strings.HasSuffix(event.Resource.Path, audithttp.TruncationSuffix))
		require.Len(t, event.Actor.UserAgent, audithttp.DefaultMaxUserAgentLength)
		require.True(t, strings.HasSuffix(event.Actor.UserAgent, audithttp.TruncationSuffix))
	})

	t.Run("configured caps", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/abcdefghijklmnop", nil)
		req.Header.Set("User-Agent", "0123456789abcdef")
		event := auditedEvent(t, req, audithttp.WithMaxPathLength(8), audithttp.WithMaxUserAgentLength(6))

		require.Equal(t, "/abcd...", event.Resource.Path)
		require.Equal(t, "012...", event.Actor.UserAgent)
	})

	t.Run("values within the caps are stored as is", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/items/42", nil)
		req.Header.Set("User-Agent", "curl/8.4.0")
		event := auditedEvent(t, req)

		require.Equal(t, "/api/v1/items/42", event.Resource.Path)
		require.Equal(t, "curl/8.4.0", event.Actor.UserAgent)
	})

	t.Run("a User-Agent from the actor extractor is capped too", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/items", nil)
		event := auditedEvent(t, req,
			audithttp.WithActorExtractor(func(*http.Request) audit.Actor {
				return audit.Actor{ID: "u1", UserAgent: strings.Repeat("x", 3*audithttp.DefaultMaxUserAgentLength)}
			}))

		require.Equal(t, "u1", event.Actor.ID)
		require.Len(t, event.Actor.UserAgent, audithttp.DefaultMaxUserAgentLength)
	})

	t.Run("a non-positive cap keeps the default", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/items/42", nil)
		req.Header.Set("User-Agent", "curl/8.4.0")
		event := auditedEvent(t, req, audithttp.WithMaxPathLength(0), audithttp.WithMaxUserAgentLength(-1))

		require.Equal(t, "/api/v1/items/42", event.Resource.Path)
		require.Equal(t, "curl/8.4.0", event.Actor.UserAgent)
	})
}
