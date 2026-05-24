// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package static_test

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/static"
)

type userInfo struct {
	ID    string
	Role  string
	Email string
}

func TestNewInMemoryStore_EmptyStore(t *testing.T) {
	t.Parallel()
	s := static.NewInMemoryStore()
	require.NotNil(t, s)
	require.Equal(t, 0, s.TokenCount())
}

func TestNewInMemoryStore_InitialTokens(t *testing.T) {
	t.Parallel()
	s := static.NewInMemoryStore(static.WithInitialTokens(map[string]any{
		"token1": userInfo{ID: "user1", Role: "admin"},
		"token2": userInfo{ID: "user2", Role: "viewer"},
	}))
	require.Equal(t, 2, s.TokenCount())
}

func TestInMemoryStore_Validate(t *testing.T) {
	t.Parallel()
	want := userInfo{ID: "user1", Role: "admin", Email: "user1@example.com"}

	cases := []struct {
		name    string
		token   string
		wantErr error
	}{
		{"valid token", "valid_token", nil},
		{"invalid token", "invalid_token", static.ErrInvalidToken},
		{"empty token", "", static.ErrEmptyToken},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := static.NewInMemoryStore(static.WithInitialTokens(map[string]any{
				"valid_token": want,
			}))
			data, err := s.Validate(t.Context(), tc.token)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.Nil(t, data)
				return
			}
			require.NoError(t, err)
			require.Equal(t, want, data)
		})
	}
}

func TestInMemoryStore_AddRemoveToken(t *testing.T) {
	t.Parallel()
	s := static.NewInMemoryStore()

	s.AddToken("token1", userInfo{ID: "user1"})
	require.Equal(t, 1, s.TokenCount())

	data, err := s.Validate(t.Context(), "token1")
	require.NoError(t, err)
	require.Equal(t, userInfo{ID: "user1"}, data)

	s.AddToken("token1", userInfo{ID: "user1-updated"})
	require.Equal(t, 1, s.TokenCount())

	data, err = s.Validate(t.Context(), "token1")
	require.NoError(t, err)
	require.Equal(t, userInfo{ID: "user1-updated"}, data)

	s.RemoveToken("token1")
	require.Equal(t, 0, s.TokenCount())

	_, err = s.Validate(t.Context(), "token1")
	require.ErrorIs(t, err, static.ErrInvalidToken)

	// Removing missing token is a no-op.
	s.RemoveToken("nonexistent")
	require.Equal(t, 0, s.TokenCount())

	// Empty token operations are no-ops.
	s.AddToken("", userInfo{})
	s.RemoveToken("")
	require.Equal(t, 0, s.TokenCount())
}

func TestInMemoryStore_WithHMACKey_StableDigests(t *testing.T) {
	t.Parallel()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	a := static.NewInMemoryStore(
		static.WithHMACKey(key),
		static.WithInitialTokens(map[string]any{"token": userInfo{ID: "u1"}}),
	)
	b := static.NewInMemoryStore(static.WithHMACKey(key))

	// Both stores derive the same digest, so adding the same token to b
	// stores it under the same key as in a — verifying our key plumbing.
	b.AddToken("token", userInfo{ID: "u1"})

	dataA, err := a.Validate(t.Context(), "token")
	require.NoError(t, err)
	dataB, err := b.Validate(t.Context(), "token")
	require.NoError(t, err)
	require.Equal(t, dataA, dataB)
}

func TestInMemoryStore_WithHMACKey_ShortKeyIgnored(t *testing.T) {
	t.Parallel()
	short := []byte("toosmall") // < 16 bytes
	s := static.NewInMemoryStore(static.WithHMACKey(short))
	s.AddToken("t", userInfo{ID: "u1"})
	// Validation still works — the short key was ignored and a random one used.
	_, err := s.Validate(t.Context(), "t")
	require.NoError(t, err)
}

func TestInMemoryStore_Concurrent(t *testing.T) {
	t.Parallel()
	s := static.NewInMemoryStore()
	const goroutines = 50
	const perGoroutine = 20

	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Go(func() {
			for j := range perGoroutine {
				token := "token_" + strconv.Itoa(i) + "_" + strconv.Itoa(j)
				s.AddToken(token, userInfo{ID: strconv.Itoa(i)})
				_, _ = s.Validate(context.Background(), token)
				if j%2 == 0 {
					s.RemoveToken(token)
				}
			}
		})
	}
	wg.Wait()

	count := s.TokenCount()
	require.GreaterOrEqual(t, count, 0)
	require.LessOrEqual(t, count, goroutines*perGoroutine)
}

// denyLimiter records every Allow / RecordFailure call so tests can assert
// the precise sequence the decorator drives.
type denyLimiter struct {
	allow      bool
	failedKeys []string
	mu         sync.Mutex
}

func (l *denyLimiter) Allow(_ context.Context, _ string) bool { return l.allow }

func (l *denyLimiter) RecordFailure(_ context.Context, key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failedKeys = append(l.failedKeys, key)
}

func (l *denyLimiter) failuresFor(key string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, k := range l.failedKeys {
		if k == key {
			n++
		}
	}
	return n
}

func TestRateLimitedStore_Validate(t *testing.T) {
	t.Parallel()

	t.Run("success does not touch limiter", func(t *testing.T) {
		t.Parallel()
		inner := static.NewInMemoryStore(static.WithInitialTokens(map[string]any{
			"good": userInfo{ID: "u1"},
		}))
		limiter := &denyLimiter{allow: true}
		s := static.NewRateLimitedStore(inner, limiter, func(context.Context) string { return "key" })

		data, err := s.Validate(t.Context(), "good")
		require.NoError(t, err)
		require.Equal(t, userInfo{ID: "u1"}, data)
		require.Empty(t, limiter.failedKeys,
			"success must NOT touch the limiter — a success-driven reset/refund is the bypass we are defending against")
	})

	t.Run("deny returns ErrRateLimited", func(t *testing.T) {
		t.Parallel()
		inner := static.NewInMemoryStore()
		limiter := &denyLimiter{allow: false}
		s := static.NewRateLimitedStore(inner, limiter, nil)

		_, err := s.Validate(t.Context(), "anything")
		require.ErrorIs(t, err, static.ErrRateLimited)
		require.NotContains(t, err.Error(), "global",
			"rate-limit key (here the default \"global\") must not leak into the error string — it propagates to logs and gRPC/HTTP error payloads")
	})

	t.Run("failed auth records failure", func(t *testing.T) {
		t.Parallel()
		inner := static.NewInMemoryStore()
		limiter := &denyLimiter{allow: true}
		s := static.NewRateLimitedStore(inner, limiter, func(context.Context) string { return "k" })

		_, err := s.Validate(t.Context(), "bad")
		require.ErrorIs(t, err, static.ErrInvalidToken)
		require.Equal(t, []string{"k"}, limiter.failedKeys,
			"failed validation must debit the per-key budget exactly once")
	})

	t.Run("empty key skips limiter", func(t *testing.T) {
		t.Parallel()
		inner := static.NewInMemoryStore(static.WithInitialTokens(map[string]any{"t": userInfo{}}))
		limiter := &denyLimiter{allow: false} // would deny if consulted
		s := static.NewRateLimitedStore(inner, limiter, func(context.Context) string { return "" })

		_, err := s.Validate(t.Context(), "t")
		require.NoError(t, err)
		require.Empty(t, limiter.failedKeys,
			"empty key short-circuits BOTH Allow and RecordFailure")
	})

	t.Run("nil store panics", func(t *testing.T) {
		t.Parallel()
		require.Panics(t, func() {
			static.NewRateLimitedStore(nil, &denyLimiter{}, nil)
		})
	})

	t.Run("nil limiter panics", func(t *testing.T) {
		t.Parallel()
		require.Panics(t, func() {
			static.NewRateLimitedStore(static.NewInMemoryStore(), nil, nil)
		})
	})

	// Regression: 1 valid + N invalid attempts must accumulate the N
	// failures in the limiter. Before the Allow / RecordFailure split the
	// successful call on each cycle reset the budget, letting an attacker
	// brute-force forever as long as they held a single valid token.
	t.Run("interleaved success does not absorb failure budget", func(t *testing.T) {
		t.Parallel()
		inner := static.NewInMemoryStore(static.WithInitialTokens(map[string]any{
			"good": userInfo{ID: "u1"},
		}))
		limiter := &denyLimiter{allow: true}
		key := "client-1"
		s := static.NewRateLimitedStore(inner, limiter, func(context.Context) string { return key })

		const cycles = 5
		for range cycles {
			_, err := s.Validate(t.Context(), "good")
			require.NoError(t, err, "valid token must keep validating")

			_, err = s.Validate(t.Context(), "bad")
			require.ErrorIs(t, err, static.ErrInvalidToken)
		}

		require.Equal(t, cycles, limiter.failuresFor(key),
			"every failed attempt must be recorded — successes do NOT clear the counter")
	})
}

func TestMetrics_NilSafe(t *testing.T) {
	t.Parallel()
	var m *static.Metrics
	m.RecordValidation(true, 0)
	m.SetActiveTokens(10)
	// no panic
}

// errStore returns a known error so we can assert on error propagation.
type errStore struct{ err error }

func (e errStore) Validate(context.Context, string) (any, error) { return nil, e.err }

func TestRateLimitedStore_PropagatesStoreError(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("custom error")
	s := static.NewRateLimitedStore(errStore{err: sentinel}, &denyLimiter{allow: true}, nil)
	_, err := s.Validate(t.Context(), "x")
	require.ErrorIs(t, err, sentinel)
}
