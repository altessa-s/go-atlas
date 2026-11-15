// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeVerifier is a TokenVerifier double returning a fixed result.
type fakeVerifier struct {
	claims map[string]any
	err    error
}

func (f *fakeVerifier) ValidateToken(context.Context, string) (map[string]any, error) {
	return f.claims, f.err
}

// levelRecorder is a slog.Handler that records the level of every log record.
type levelRecorder struct {
	mu     sync.Mutex
	levels []slog.Level
}

func (r *levelRecorder) Enabled(context.Context, slog.Level) bool { return true }

func (r *levelRecorder) Handle(_ context.Context, rec slog.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.levels = append(r.levels, rec.Level)
	return nil
}

func (r *levelRecorder) WithAttrs([]slog.Attr) slog.Handler { return r }
func (r *levelRecorder) WithGroup(string) slog.Handler      { return r }

func (r *levelRecorder) recorded() []slog.Level {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.levels)
}

// A rejected token is an everyday event any caller can provoke, so it is
// logged at Debug; only infrastructure failures reach Error.
func TestValidator_ValidateToken_LogLevel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		cause error
		want  slog.Level
	}{
		{name: "invalid token", cause: fmt.Errorf("%w: expired", ErrTokenInvalid), want: slog.LevelDebug},
		{name: "revoked token", cause: ErrTokenRevoked, want: slog.LevelDebug},
		{name: "failed CEL rule", cause: fmt.Errorf("%w: rule 'x' failed", ErrCELValidation), want: slog.LevelDebug},
		{name: "stale JWKS", cause: ErrJWKSStale, want: slog.LevelError},
		{name: "revocation storage down", cause: ErrRevocationCheck, want: slog.LevelError},
		{name: "introspection down", cause: ErrIntrospection, want: slog.LevelError},
		{name: "unclassified", cause: errors.New("boom"), want: slog.LevelError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := &levelRecorder{}
			v := NewValidator(&fakeVerifier{err: tc.cause}, WithValidatorLogger(slog.New(rec)))

			_, err := v.ValidateToken(t.Context(), "token")
			require.ErrorIs(t, err, tc.cause)
			require.Equal(t, []slog.Level{tc.want}, rec.recorded())
		})
	}
}

func TestProvider_RejectedTokenIsNotLoggedAtError(t *testing.T) {
	t.Parallel()
	const expression = "claims.role == 'admin'"

	idp := newTestIdP(t)
	rec := &levelRecorder{}
	p := idp.newProvider(t, WithLogger(slog.New(rec)))
	raw := idp.sign(t, idp.claims(map[string]any{"role": "user"}))

	_, err := p.ValidateTokenWithOptions(t.Context(), raw,
		WithValidationAudience(testAudience),
		WithValidationCelRules(CELValidationRule{Name: "admin-only", Expression: expression}))
	require.ErrorIs(t, err, ErrCELValidation)
	require.NotContains(t, err.Error(), expression, "the rule expression must not leak into the error")
	require.ErrorContains(t, err, "admin-only")

	_, err = p.ValidateTokenWithOptions(t.Context(), raw, WithValidationAudience("other"))
	require.ErrorIs(t, err, ErrTokenInvalid)

	require.NotContains(t, rec.recorded(), slog.LevelError)
	require.Contains(t, rec.recorded(), slog.LevelDebug)
}

func TestValidator_ValidateToken_Success(t *testing.T) {
	t.Parallel()
	v := NewValidator(&fakeVerifier{claims: map[string]any{"sub": "user1", "iss": "https://issuer"}})

	claims, err := v.ValidateToken(t.Context(), "valid-token")
	require.NoError(t, err)
	require.Equal(t, "user1", claims.Subject)
	require.Equal(t, "https://issuer", claims.Issuer)
}

func TestValidator_ValidateToken_ErrorKeepsCause(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		cause error
	}{
		{name: "invalid", cause: ErrTokenInvalid},
		{name: "revoked", cause: ErrTokenRevoked},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			v := NewValidator(&fakeVerifier{err: tc.cause})

			claims, err := v.ValidateToken(t.Context(), "bad-token")
			require.Nil(t, claims)
			require.ErrorIs(t, err, tc.cause)
		})
	}
}
