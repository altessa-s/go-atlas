// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/audit/decision"
	"github.com/altessa-s/go-atlas/auth/principal"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth/oidc"

	authoidc "github.com/altessa-s/go-atlas/auth/oidc"
)

// fakeValidator is an authoidc.ClaimsValidator double returning a fixed result.
type fakeValidator struct {
	claims *authoidc.Claims
	err    error
}

func (f fakeValidator) ValidateToken(context.Context, string) (*authoidc.Claims, error) {
	return f.claims, f.err
}

// captureSink records every decision it receives for later assertions.
type captureSink struct {
	mu        sync.Mutex
	decisions []decision.Decision
	err       error
}

func (s *captureSink) Record(_ context.Context, d decision.Decision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.decisions = append(s.decisions, d)
	return s.err
}

func (s *captureSink) all() []decision.Decision {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]decision.Decision(nil), s.decisions...)
}

func subjectOf(c *authoidc.Claims) string { return c.Subject }

// serve runs one request carrying a bearer token through the auth middleware
// wired with fn and returns the recorder and the data the handler observed.
func serve(t *testing.T, fn auth.AuthFunc) (*httptest.ResponseRecorder, any) {
	t.Helper()
	var seen any
	handler := auth.Middleware(auth.WithAuthFunc(fn))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = auth.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	req.Header.Set("Authorization", "Bearer some-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec, seen
}

func TestAuthFunc_Success(t *testing.T) {
	t.Parallel()
	want := &authoidc.Claims{Subject: "user1", Scopes: []string{"files:read"}}

	rec, seen := serve(t, oidc.AuthFunc(fakeValidator{claims: want}))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Same(t, want, seen)
}

func TestAuthFunc_FailureIsUnauthorized(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		cause error
	}{
		{name: "invalid", cause: authoidc.ErrTokenInvalid},
		{name: "revoked", cause: authoidc.ErrTokenRevoked},
		{name: "other", cause: errors.New("boom")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fn := oidc.AuthFunc(fakeValidator{err: tc.cause})

			_, err := fn.Authenticate(t.Context(), "bad")
			require.ErrorIs(t, err, auth.ErrUnauthorized)
			require.ErrorIs(t, err, tc.cause)

			rec, seen := serve(t, fn)
			require.Equal(t, http.StatusUnauthorized, rec.Code)
			require.Nil(t, seen)
		})
	}
}

func TestAuthFunc_NilValidatorPanics(t *testing.T) {
	t.Parallel()
	require.Panics(t, func() { oidc.AuthFunc(nil) })
}

func TestAuthFunc_Audit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		validator   fakeValidator
		wantAllowed bool
		wantReason  string
		wantSubject string
	}{
		{name: "allowed", validator: fakeValidator{claims: &authoidc.Claims{Subject: "user1"}}, wantAllowed: true, wantSubject: "user1"},
		{name: "invalid", validator: fakeValidator{err: authoidc.ErrTokenInvalid}, wantReason: "invalid_token"},
		{name: "revoked", validator: fakeValidator{err: authoidc.ErrTokenRevoked}, wantReason: "revoked"},
		{name: "other", validator: fakeValidator{err: errors.New("boom")}, wantReason: "error"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sink := &captureSink{}
			rec := decision.NewRecorder(sink, decision.WithPolicyMode(decision.PolicyAll))
			fn := oidc.AuthFunc(tc.validator, oidc.WithAudit(rec, subjectOf))

			_, _ = fn.Authenticate(t.Context(), "tok")

			decisions := sink.all()
			require.Len(t, decisions, 1)
			d := decisions[0]
			require.Equal(t, tc.wantAllowed, d.Allowed)
			require.Equal(t, tc.wantReason, d.Reason)
			require.Equal(t, tc.wantSubject, d.Subject)
			require.Equal(t, "validate_token", d.Action)
			require.Equal(t, "http", d.Attributes["transport"])
		})
	}
}

func TestAuthFunc_AuditFailureRequiredFailsClosed(t *testing.T) {
	t.Parallel()
	sink := &captureSink{err: errors.New("sink unavailable")}
	rec := decision.NewRecorder(sink, decision.WithPolicyMode(decision.PolicyAll), decision.WithFailureMode(decision.FailureRequired))
	fn := oidc.AuthFunc(fakeValidator{claims: &authoidc.Claims{Subject: "user1"}}, oidc.WithAudit(rec, subjectOf))

	data, err := fn.Authenticate(t.Context(), "tok")
	require.Error(t, err)
	require.Nil(t, data)
}

// TestAuthFunc_BridgesPrincipal pins that the OIDC path exposes the caller
// through principal.FromContext, as every other authenticator does, while
// auth.FromContext keeps returning the *oidc.Claims.
func TestAuthFunc_BridgesPrincipal(t *testing.T) {
	t.Parallel()
	claims := &authoidc.Claims{
		Subject:   "user1",
		Scopes:    []string{"files:read"},
		RawClaims: map[string]any{"sub": "user1", "tenant": "acme", "roles": []any{"admin"}},
	}

	var (
		got  principal.Principal
		ok   bool
		data any
	)
	handler := auth.Middleware(auth.WithAuthFunc(oidc.AuthFunc(fakeValidator{claims: claims})))(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got, ok = principal.FromContext(r.Context())
			data = auth.FromContext(r.Context())
			w.WriteHeader(http.StatusOK)
		}))
	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	req.Header.Set("Authorization", "Bearer some-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, ok, "principal.FromContext must be set on the OIDC path")
	require.Equal(t, principal.Principal{
		Subject: "user1", Tenant: "acme", Scopes: []string{"files:read"}, Roles: []string{"admin"},
	}, got)
	require.Same(t, claims, data)
}
