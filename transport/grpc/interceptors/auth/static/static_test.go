// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package static_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/static"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authstatic "github.com/altessa-s/go-atlas/auth/static"
)

type userInfo struct {
	ID   string
	Role string
}

func TestAuthFunc_Success(t *testing.T) {
	t.Parallel()
	want := userInfo{ID: "user1", Role: "admin"}
	store := static.NewInMemoryStore(static.WithInitialTokens(map[string]any{
		"valid_token": want,
	}))
	fn := static.AuthFunc(store)

	data, err := fn(t.Context(), auth.Request{
		Base:    auth.Base{AuthMethod: auth.MethodToken},
		Payload: &auth.TokenCredentials{Token: "valid_token"},
	})
	require.NoError(t, err)
	require.Equal(t, want, data)
}

func TestAuthFunc_ErrorMapping(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		store    static.TokenStore
		req      auth.Request
		wantCode codes.Code
		wantMsg  string
	}{
		{
			name:     "missing token credentials",
			store:    static.NewInMemoryStore(),
			req:      auth.Request{Base: auth.Base{AuthMethod: "other"}},
			wantCode: codes.Unauthenticated,
			wantMsg:  "missing token credentials",
		},
		{
			name:     "invalid token",
			store:    static.NewInMemoryStore(),
			req:      auth.Request{Base: auth.Base{AuthMethod: auth.MethodToken}, Payload: &auth.TokenCredentials{Token: "bad"}},
			wantCode: codes.Unauthenticated,
			wantMsg:  "invalid token",
		},
		{
			name:     "empty token",
			store:    static.NewInMemoryStore(),
			req:      auth.Request{Base: auth.Base{AuthMethod: auth.MethodToken}, Payload: &auth.TokenCredentials{Token: ""}},
			wantCode: codes.Unauthenticated,
			wantMsg:  "invalid token",
		},
		{
			name:     "rate limited",
			store:    static.NewRateLimitedStore(static.NewInMemoryStore(), denyLimiter{}, nil),
			req:      auth.Request{Base: auth.Base{AuthMethod: auth.MethodToken}, Payload: &auth.TokenCredentials{Token: "anything"}},
			wantCode: codes.ResourceExhausted,
		},
		{
			name:     "internal error",
			store:    errStore{err: errors.New("boom")},
			req:      auth.Request{Base: auth.Base{AuthMethod: auth.MethodToken}, Payload: &auth.TokenCredentials{Token: "x"}},
			wantCode: codes.Internal,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fn := static.AuthFunc(tc.store)
			data, err := fn(t.Context(), tc.req)
			require.Error(t, err)
			require.Nil(t, data)

			st, ok := status.FromError(err)
			require.True(t, ok)
			require.Equal(t, tc.wantCode, st.Code())
			if tc.wantMsg != "" {
				require.Contains(t, st.Message(), tc.wantMsg)
			}
		})
	}
}

func TestAuthFunc_NilStorePanics(t *testing.T) {
	t.Parallel()
	require.Panics(t, func() { static.AuthFunc(nil) })
}

type denyLimiter struct{}

func (denyLimiter) Allow(context.Context, string) bool    { return false }
func (denyLimiter) RecordFailure(context.Context, string) {}

type errStore struct{ err error }

func (e errStore) Validate(context.Context, string) (any, error) { return nil, e.err }

// Ensure auth/static error sentinels remain visible to callers of this
// package — adapters above this layer rely on errors.Is across the boundary.
func TestErrorSentinelsAcrossPackage(t *testing.T) {
	t.Parallel()
	require.True(t, errors.Is(authstatic.ErrInvalidToken, authstatic.ErrInvalidToken))
}
