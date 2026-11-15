// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth_test

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/security/vault/auth"

	vaultApi "github.com/hashicorp/vault/api"
)

// outageMethod fails with a retryable error except on the attempts listed in
// succeedOn, where it issues a non-renewable one-hour token. It records when
// each attempt started.
type outageMethod struct {
	succeedOn map[int]bool
	mu        sync.Mutex
	attempts  []time.Time
	shutdown  bool
}

func (m *outageMethod) Authenticate(context.Context, *vaultApi.Client) (*vaultApi.Secret, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := len(m.attempts)
	m.attempts = append(m.attempts, time.Now())
	if m.succeedOn[n] {
		return &vaultApi.Secret{Auth: &vaultApi.SecretAuth{ClientToken: "t", LeaseDuration: 3600}}, nil
	}
	return nil, errors.New("dial tcp: connection refused")
}

func (m *outageMethod) Shutdown() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shutdown = true
	return nil
}
func (*outageMethod) Name() string      { return "outage" }
func (*outageMethod) IssuesToken() bool { return true }

func (m *outageMethod) gaps() []time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	gaps := make([]time.Duration, 0, len(m.attempts))
	for i := 1; i < len(m.attempts); i++ {
		gaps = append(gaps, m.attempts[i].Sub(m.attempts[i-1]))
	}
	return gaps
}

// During an outage the delay between logins must grow from the base to the
// configured maximum instead of staying at the base, and a successful login
// must reset it.
func TestRun_FailureBackoffGrowsToMaxAndResetsOnSuccess(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
		require.NoError(t, err)
		// Attempts 0-6 fail, 7 succeeds, 8 fails, 9 succeeds.
		m := &outageMethod{succeedOn: map[int]bool{7: true, 9: true}}
		a := auth.NewAuthenticator(client, m, auth.WithLogger(slog.New(slog.DiscardHandler)))

		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- a.Run(ctx) }()
		time.Sleep(2 * time.Hour) // fake time
		cancel()
		require.NoError(t, <-done)

		gaps := m.gaps()
		require.GreaterOrEqual(t, len(gaps), 9)
		// Delay after the n-th consecutive failure: base*2^n plus up to as much
		// jitter, capped at the maximum.
		for n, gap := range gaps[:7] {
			floor := min(auth.DefaultBackoffBase<<n, auth.DefaultBackoffMax)
			require.GreaterOrEqual(t, gap, floor, "failure %d", n)
			require.LessOrEqual(t, gap, min(2*floor, auth.DefaultBackoffMax), "failure %d", n)
		}
		require.Equal(t, auth.DefaultBackoffMax, gaps[4], "2s*2^4 exceeds the maximum")
		require.Equal(t, 40*time.Minute, gaps[7], "a success is refreshed on the lease schedule, not the backoff")
		require.GreaterOrEqual(t, gaps[8], auth.DefaultBackoffBase, "the backoff starts over after a success")
		require.LessOrEqual(t, gaps[8], 2*auth.DefaultBackoffBase)
	})
}

// WithKeepMethodOnExit leaves the method's credentials alone when Run returns.
func TestRun_KeepMethodOnExit(t *testing.T) {
	t.Parallel()
	for _, keep := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
			require.NoError(t, err)
			m := &outageMethod{}
			opts := []auth.Option{auth.WithLogger(slog.New(slog.DiscardHandler))}
			if keep {
				opts = append(opts, auth.WithKeepMethodOnExit())
			}
			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			require.NoError(t, auth.NewAuthenticator(client, m, opts...).Run(ctx))

			require.Equal(t, !keep, m.shutdown)
		})
	}
}

// WithOnLogin fires on the run's goroutine for every successful login.
func TestRun_OnLoginFiresPerLogin(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
		require.NoError(t, err)
		m := &outageMethod{succeedOn: map[int]bool{0: true, 1: true}}
		logins := 0
		a := auth.NewAuthenticator(client, m,
			auth.WithLogger(slog.New(slog.DiscardHandler)), auth.WithOnLogin(func() { logins++ }))

		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- a.Run(ctx) }()
		time.Sleep(41 * time.Minute) // fake time: one refresh of the one-hour token
		cancel()
		require.NoError(t, <-done)

		require.Equal(t, 2, logins)
	})
}
