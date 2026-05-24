// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

type testUserInfo struct {
	ID   string
	Role string
}

func TestMiddleware_Dependencies(t *testing.T) {
	t.Parallel()
	m := New()
	require.Empty(t, m.Dependencies())
}

func TestMiddleware_Name(t *testing.T) {
	t.Parallel()
	m := New()
	require.Equal(t, "auth", m.Name())
}

func TestMiddleware_Authenticate_Success(t *testing.T) {
	t.Parallel()
	want := testUserInfo{ID: "user1", Role: "admin"}

	m := New(
		WithAuthFunc(AuthenticateFunc(func(_ context.Context, token string) (any, error) {
			if token == "valid_token" {
				return want, nil
			}
			return nil, ErrUnauthorized
		})),
		WithTokenExtractor(TokenExtractorFunc(func(r *http.Request) (string, error) {
			h := r.Header.Get("Authorization")
			if h == "Bearer valid_token" {
				return "valid_token", nil
			}
			return "", ErrMissingToken
		})),
	)

	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data := FromContext(r.Context())
		info, ok := data.(testUserInfo)
		require.True(t, ok)
		require.Equal(t, want, info)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer valid_token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "OK", rec.Body.String())
}

func TestMiddleware_Authenticate_InvalidToken(t *testing.T) {
	t.Parallel()
	m := New(
		WithAuthFunc(AuthenticateFunc(func(context.Context, string) (any, error) {
			return nil, ErrUnauthorized
		})),
	)

	handler := m.Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler must not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer invalid_token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Contains(t, rec.Body.String(), "Authentication failed")
}

func TestMiddleware_Authenticate_MissingToken(t *testing.T) {
	t.Parallel()
	m := New(
		WithAuthFunc(AuthenticateFunc(func(context.Context, string) (any, error) {
			t.Fatal("auth func must not be called")
			return nil, nil
		})),
	)

	handler := m.Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler must not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Equal(t, `Bearer realm="Restricted"`, rec.Header().Get("WWW-Authenticate"))
	require.Contains(t, rec.Body.String(), "Missing or invalid authorization")
}

func TestMiddleware_Authenticate_WrappedMissingToken(t *testing.T) {
	t.Parallel()
	wrapped := errors.Join(ErrMissingToken, errors.New("upstream context"))

	m := New(
		WithTokenExtractor(TokenExtractorFunc(func(*http.Request) (string, error) {
			return "", wrapped
		})),
		WithAuthFunc(AuthenticateFunc(func(context.Context, string) (any, error) { return nil, nil })),
	)

	handler := m.Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler must not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Default handler must still recognize the wrapped sentinel and set
	// WWW-Authenticate — this is the regression for the prior `err ==
	// ErrMissingToken` check.
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Equal(t, `Bearer realm="Restricted"`, rec.Header().Get("WWW-Authenticate"))
}

func TestMiddleware_IgnorePaths(t *testing.T) {
	t.Parallel()
	m := New(
		WithIgnorePaths("/health", "/metrics"),
		WithAuthFunc(AuthenticateFunc(func(context.Context, string) (any, error) {
			t.Fatal("auth func must not be called for ignored paths")
			return nil, nil
		})),
	)

	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	req = httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestMiddleware_CustomErrorHandler(t *testing.T) {
	t.Parallel()
	customErr := errors.New("custom auth error")

	m := New(
		WithAuthFunc(AuthenticateFunc(func(context.Context, string) (any, error) {
			return nil, customErr
		})),
		WithErrorHandler(ErrorHandlerFunc(func(w http.ResponseWriter, _ *http.Request, err error) {
			if errors.Is(err, customErr) {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte("Custom error response"))
			}
		})),
	)

	handler := m.Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler must not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Equal(t, "Custom error response", rec.Body.String())
}

func TestExtractBearerToken(t *testing.T) {
	t.Parallel()
	extractor := ExtractBearerToken()

	cases := []struct {
		name       string
		authHeader string
		wantToken  string
		wantErr    bool
	}{
		{"valid bearer token", "Bearer abc123", "abc123", false},
		{"case insensitive bearer", "bearer xyz789", "xyz789", false},
		{"missing bearer prefix", "Token abc123", "", true},
		{"empty header", "", "", true},
		{"bearer with extra spaces", "Bearer   token123  ", "token123", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			token, err := extractor.ExtractToken(req)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantToken, token)
		})
	}
}

func TestExtractTokenFromHeader(t *testing.T) {
	t.Parallel()
	extractor := ExtractTokenFromHeader("X-API-Key", func(value string) (string, error) {
		if value == "" {
			return "", ErrInvalidToken
		}
		return value, nil
	})

	cases := []struct {
		name      string
		header    string
		wantToken string
		wantErr   bool
	}{
		{"valid api key", "secret_key_123", "secret_key_123", false},
		{"empty header", "", "", true},
		{"whitespace trimmed", "  key456  ", "key456", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.header != "" {
				req.Header.Set("X-API-Key", tc.header)
			}
			token, err := extractor.ExtractToken(req)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantToken, token)
		})
	}
}

func TestFromContext(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	require.Nil(t, FromContext(ctx))

	want := testUserInfo{ID: "user1", Role: "admin"}
	ctx = context.WithValue(ctx, authContextKey, want)
	require.Equal(t, want, FromContext(ctx))
}

func TestMustFromContext(t *testing.T) {
	t.Parallel()
	require.Panics(t, func() {
		MustFromContext(t.Context())
	})

	want := testUserInfo{ID: "user1", Role: "admin"}
	ctx := context.WithValue(t.Context(), authContextKey, want)
	require.Equal(t, want, MustFromContext(ctx))
}

func TestMiddleware_NilAuthFunc_Rejects(t *testing.T) {
	t.Parallel()
	m := New()
	handler := m.Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler must not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer x")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}
