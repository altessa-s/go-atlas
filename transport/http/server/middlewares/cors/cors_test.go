// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cors

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	m := New()
	require.Equal(t, "cors", m.Name())
}

func TestMiddleware_NoCORSRequest(t *testing.T) {
	handler := Middleware(WithAllowAllOrigins())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	handler.ServeHTTP(rec, req)

	require.Equal(t, "", rec.Header().Get(HeaderAccessControlAllowOrigin))
}

func TestMiddleware_AllowAllOrigins(t *testing.T) {
	handler := Middleware(WithAllowAllOrigins())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	handler.ServeHTTP(rec, req)

	require.Equal(t, "*", rec.Header().Get(HeaderAccessControlAllowOrigin))
}

func TestMiddleware_SpecificOrigin(t *testing.T) {
	handler := Middleware(WithAllowedOrigins("https://example.com"))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("allowed", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Origin", "https://example.com")
		handler.ServeHTTP(rec, req)
		require.Equal(t, "https://example.com", rec.Header().Get(HeaderAccessControlAllowOrigin))
	})

	t.Run("not_allowed", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Origin", "https://evil.com")
		handler.ServeHTTP(rec, req)
		require.Equal(t, "", rec.Header().Get(HeaderAccessControlAllowOrigin))
	})
}

func TestMiddleware_Preflight(t *testing.T) {
	handler := Middleware(
		WithAllowAllOrigins(),
		WithAllowedMethods("GET", "POST"),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Fail(t, "next handler should not be called for preflight")
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("OPTIONS", "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.NotEqual(t, "", rec.Header().Get(HeaderAccessControlAllowMethods))
}

func TestMiddleware_Credentials(t *testing.T) {
	handler := Middleware(
		WithAllowedOrigins("https://example.com"),
		WithAllowCredentials(),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	handler.ServeHTTP(rec, req)

	require.Equal(t, "true", rec.Header().Get(HeaderAccessControlAllowCredentials))
	// With credentials, origin should be echoed, not *
	require.Equal(t, "https://example.com", rec.Header().Get(HeaderAccessControlAllowOrigin))
}

func TestMiddleware_MaxAge(t *testing.T) {
	handler := Middleware(
		WithAllowAllOrigins(),
		WithMaxAge(3600),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("OPTIONS", "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	handler.ServeHTTP(rec, req)

	require.Equal(t, "3600", rec.Header().Get(HeaderAccessControlMaxAge))
}

func TestMiddleware_ExposedHeaders(t *testing.T) {
	handler := Middleware(
		WithAllowAllOrigins(),
		WithExposedHeaders("X-Custom"),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	handler.ServeHTTP(rec, req)

	require.Equal(t, "X-Custom", rec.Header().Get(HeaderAccessControlExposeHeaders))
}

func TestMiddleware_Dependencies(t *testing.T) {
	m := New()
	require.Nil(t, m.Dependencies())
}

func TestNew_AllowAllOriginsWithCredentials_Panics(t *testing.T) {
	defer func() {
		r := recover()
		require.NotNil(t, r)
		msg, ok := r.(string)
		require.True(t, ok && strings.Contains(msg, "insecure configuration"), "unexpected panic: %v", r)
	}()

	New(WithAllowAllOrigins(), WithAllowCredentials())
}

func TestNew_AllowAllOrigins_NoPanic(t *testing.T) {
	m := New(WithAllowAllOrigins())
	require.NotNil(t, m)
}

func TestNew_AllowCredentials_NoPanic(t *testing.T) {
	m := New(WithAllowedOrigins("https://example.com"), WithAllowCredentials())
	require.NotNil(t, m)
}

// TestNew_BroadOriginPatternWithCredentials_Panics pins the fix: a regex that
// matches arbitrary origins is as dangerous as AllowAllOrigins when combined with
// credentials, and must be rejected the same way.
func TestNew_BroadOriginPatternWithCredentials_Panics(t *testing.T) {
	for _, tc := range []struct {
		name    string
		pattern string
	}{
		{"match everything", `.*`},
		{"any scheme+host", `^https?://.*$`},
		{"empty pattern", ``},
		// Not anchored at the end: also matches https://x.example.com.evil.io.
		{"suffix-unanchored subdomain", `^https://.*\.example\.com`},
		{"suffix-unanchored alternative", `^https://a\.example\.com$|^https://b\.example\.com`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				r := recover()
				require.NotNil(t, r, "expected panic for broad pattern %q", tc.pattern)
				msg, ok := r.(string)
				require.True(t, ok && strings.Contains(msg, "insecure configuration"), "unexpected panic: %v", r)
			}()
			New(WithAllowedOriginPatterns(regexp.MustCompile(tc.pattern)), WithAllowCredentials())
		})
	}
}

// TestNew_NarrowOriginPatternWithCredentials_NoPanic verifies a domain-anchored
// pattern + credentials is allowed and still matches its own origins.
func TestNew_NarrowOriginPatternWithCredentials_NoPanic(t *testing.T) {
	m := New(
		WithAllowedOriginPatterns(regexp.MustCompile(`^https://[a-z0-9-]+\.example\.com$`)),
		WithAllowCredentials(),
	)
	require.NotNil(t, m)
	require.True(t, m.isOriginAllowed("https://app.example.com"))
	require.False(t, m.isOriginAllowed("https://evil.invalid"))
}

func TestNew_StartUnanchoredOriginPatternWithCredentials_Panics(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		pattern string
	}{
		{"missing start anchor", `example\.com$`},
		{"alternation second unanchored", `^https://a\.com$|b\.com$`},
		{"alternation first unanchored", `a\.com$|^https://b\.com$`},
		{"captured unanchored", `(example\.com$)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Panics(t, func() {
				New(WithAllowCredentials(), WithAllowedOriginPatterns(regexp.MustCompile(tc.pattern)))
			})
		})
	}

	t.Run("guard anchored patterns accepted", func(t *testing.T) {
		t.Parallel()
		for _, p := range []string{
			`^https://app\.example\.com$`,
			`^https://a\.example\.com$|^https://b\.example\.com$`,
			`^(https://a\.example\.com|https://b\.example\.com)$`,
			`^https://.*example\.com$`,
		} {
			require.NotPanics(t, func() {
				New(WithAllowCredentials(), WithAllowedOriginPatterns(regexp.MustCompile(p)))
			}, p)
		}
	})
}

func TestNew_CredentialedHostWildcardBoundaries(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		pattern string
		allowed bool
	}{
		{`^https://app.*\.example\.com$`, false},
		{`^https://app.+\.example\.com$`, false},
		{`^https://app.\.example\.com$`, false},
		{`^https://app[^.]*\.example\.com$`, false},
		{`^https://(app\.|app).*\.example\.com$`, false},
		{`^https://app(\.)?.*\.example\.com$`, false},
		{`^https://.*\.example\.com$`, true},
		{`^https://app\..*\.example\.com$`, true},
		{`^https://(app\.|dev\.).*\.example\.com$`, true},
		{`^https?://([a-z-]+\.)?example\.com(:[0-9]+)?$`, true},
	} {
		t.Run(tc.pattern, func(t *testing.T) {
			t.Parallel()
			options := WithAllowedOriginPatterns(regexp.MustCompile(tc.pattern))
			require.NotPanics(t, func() { New(options) })
			construct := func() { New(options, WithAllowCredentials()) }
			if tc.allowed {
				require.NotPanics(t, construct)
				return
			}
			require.Panics(t, construct)
		})
	}
}

func TestNew_CredentialedExactOriginsRejectStars(t *testing.T) {
	t.Parallel()

	for _, origin := range []string{"*", "https://app*.example.com"} {
		t.Run(origin, func(t *testing.T) {
			t.Parallel()
			defer func() {
				err, ok := recover().(error)
				require.True(t, ok, "expected an error panic")
				require.ErrorIs(t, err, ErrCredentialedOriginWildcard)
			}()
			New(WithAllowedOrigins(origin), WithAllowCredentials())
		})
	}
}

// TestMiddleware_VaryOrigin pins that every response whose CORS headers depend
// on the request Origin carries Vary: Origin, so a shared cache never serves
// one origin's (or a no-Origin request's) response to another origin.
func TestMiddleware_VaryOrigin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		opts   []Option
		method string
		origin string
	}{
		{"no origin", []Option{WithAllowedOrigins("https://a.example.com")}, http.MethodGet, ""},
		{"disallowed origin", []Option{WithAllowedOrigins("https://a.example.com")}, http.MethodGet, "https://evil.invalid"},
		{"allowed origin", []Option{WithAllowedOrigins("https://a.example.com")}, http.MethodGet, "https://a.example.com"},
		{"disallowed preflight", []Option{WithAllowedOrigins("https://a.example.com")}, http.MethodOptions, "https://evil.invalid"},
		{"allowed preflight", []Option{WithAllowedOrigins("https://a.example.com")}, http.MethodOptions, "https://a.example.com"},
		{"allow all, no origin", []Option{WithAllowAllOrigins()}, http.MethodGet, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			handler := New(tc.opts...).Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			req := httptest.NewRequest(tc.method, "/api", nil)
			if tc.origin != "" {
				req.Header.Set(HeaderOrigin, tc.origin)
			}
			if tc.method == http.MethodOptions {
				req.Header.Set(HeaderAccessControlRequestMethod, http.MethodGet)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			vary := rec.Header().Values(HeaderVary)
			require.Equal(t, 1, countValue(vary, HeaderOrigin), "want exactly one Vary: Origin, got %q", vary)
		})
	}
}

func countValue(values []string, want string) int {
	n := 0
	for _, v := range values {
		if v == want {
			n++
		}
	}
	return n
}
