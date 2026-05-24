// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package static_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth/static"

	authstatic "github.com/altessa-s/go-atlas/auth/static"
)

type userInfo struct {
	ID    string
	Role  string
	Email string
}

func TestAuthFunc_Success(t *testing.T) {
	t.Parallel()
	want := userInfo{ID: "user1", Role: "admin"}
	store := static.NewInMemoryStore(static.WithInitialTokens(map[string]any{
		"valid_token": want,
	}))
	fn := static.AuthFunc(store)

	data, err := fn.Authenticate(t.Context(), "valid_token")
	require.NoError(t, err)
	require.Equal(t, want, data)
}

func TestAuthFunc_ErrorWrapping(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name          string
		store         static.TokenStore
		token         string
		wantSentinels []error
	}{
		{
			name:          "invalid token",
			store:         static.NewInMemoryStore(),
			token:         "bad",
			wantSentinels: []error{auth.ErrUnauthorized, authstatic.ErrInvalidToken},
		},
		{
			name:          "empty token",
			store:         static.NewInMemoryStore(),
			token:         "",
			wantSentinels: []error{auth.ErrUnauthorized, authstatic.ErrEmptyToken},
		},
		{
			name:          "rate limited",
			store:         static.NewRateLimitedStore(static.NewInMemoryStore(), denyLimiter{}, nil),
			token:         "x",
			wantSentinels: []error{auth.ErrUnauthorized, authstatic.ErrRateLimited},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fn := static.AuthFunc(tc.store)
			data, err := fn.Authenticate(t.Context(), tc.token)
			require.Error(t, err)
			require.Nil(t, data)
			for _, s := range tc.wantSentinels {
				require.ErrorIs(t, err, s, "expected %v to be wrapped", s)
			}
		})
	}
}

func TestAuthFunc_NilStorePanics(t *testing.T) {
	t.Parallel()
	require.Panics(t, func() { static.AuthFunc(nil) })
}

func TestAuthFunc_InternalErrorWrapped(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("internal store failure")
	fn := static.AuthFunc(errStore{err: sentinel})

	_, err := fn.Authenticate(t.Context(), "x")
	require.ErrorIs(t, err, sentinel)
	// Not wrapped with the public Unauthorized sentinel — opaque internal errors
	// should bubble up as-is rather than masking themselves as auth failures.
	require.NotErrorIs(t, err, auth.ErrUnauthorized)
}

func TestHTTPIntegration_BearerToken(t *testing.T) {
	t.Parallel()
	want := userInfo{ID: "user1", Role: "admin"}
	store := static.NewInMemoryStore(static.WithInitialTokens(map[string]any{
		"secret_token": want,
	}))

	middleware := auth.Middleware(auth.WithAuthFunc(static.AuthFunc(store)))

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data := auth.FromContext(r.Context())
		require.Equal(t, want, data)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Authenticated: " + data.(userInfo).ID))
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	req.Header.Set("Authorization", "Bearer secret_token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "Authenticated: user1", rec.Body.String())

	req = httptest.NewRequest(http.MethodGet, "/api/data", nil)
	req.Header.Set("Authorization", "Bearer wrong_token")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHTTPIntegration_APIKey(t *testing.T) {
	t.Parallel()
	want := userInfo{ID: "service1", Role: "api_client"}
	store := static.NewInMemoryStore(static.WithInitialTokens(map[string]any{
		"api_key_123": want,
	}))

	extractor := auth.ExtractTokenFromHeader("X-API-Key", func(v string) (string, error) {
		if v == "" {
			return "", auth.ErrInvalidToken
		}
		return v, nil
	})

	middleware := auth.Middleware(
		auth.WithTokenExtractor(extractor),
		auth.WithAuthFunc(static.AuthFunc(store)),
	)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data := auth.FromContext(r.Context())
		require.Equal(t, want, data)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Service: " + data.(userInfo).ID))
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	req.Header.Set("X-API-Key", "api_key_123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "Service: service1", rec.Body.String())

	req = httptest.NewRequest(http.MethodGet, "/api/data", nil)
	req.Header.Set("X-API-Key", "wrong_key")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHTTPIntegration_DynamicTokenManagement(t *testing.T) {
	t.Parallel()
	store := static.NewInMemoryStore()
	middleware := auth.Middleware(auth.WithAuthFunc(static.AuthFunc(store)))

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	req.Header.Set("Authorization", "Bearer dynamic_token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	store.AddToken("dynamic_token", userInfo{ID: "user2"})

	req = httptest.NewRequest(http.MethodGet, "/api/data", nil)
	req.Header.Set("Authorization", "Bearer dynamic_token")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	store.RemoveToken("dynamic_token")

	req = httptest.NewRequest(http.MethodGet, "/api/data", nil)
	req.Header.Set("Authorization", "Bearer dynamic_token")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

type denyLimiter struct{}

func (denyLimiter) Allow(context.Context, string) bool { return false }
func (denyLimiter) Reset(string)                       {}

type errStore struct{ err error }

func (e errStore) Validate(context.Context, string) (any, error) { return nil, e.err }
