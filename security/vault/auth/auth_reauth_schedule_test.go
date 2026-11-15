// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/security/vault/auth"

	vaultApi "github.com/hashicorp/vault/api"
)

// timedMethod issues a non-renewable token with a one-second TTL, takes
// latency to do so, and records when each login started.
type timedMethod struct {
	latency time.Duration
	mu      sync.Mutex
	logins  []time.Time
}

func (m *timedMethod) Authenticate(context.Context, *vaultApi.Client) (*vaultApi.Secret, error) {
	m.mu.Lock()
	m.logins = append(m.logins, time.Now())
	m.mu.Unlock()
	time.Sleep(m.latency) // fake time inside the synctest bubble
	return &vaultApi.Secret{Auth: &vaultApi.SecretAuth{ClientToken: "t", LeaseDuration: 1}}, nil
}

func (*timedMethod) Shutdown() error   { return nil }
func (*timedMethod) Name() string      { return "timed" }
func (*timedMethod) IssuesToken() bool { return true }

func (m *timedMethod) Logins() []time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]time.Time(nil), m.logins...)
}

// runFor runs the authenticator for d of fake time and returns after Run exits.
func runFor(t *testing.T, a *auth.Authenticator, d time.Duration) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()
	time.Sleep(d)
	cancel()
	require.NoError(t, <-done)
}

// With the default failure backoff (2s base) a healthy Vault issuing a
// one-second non-renewable token must be asked for the next token before the
// current one expires: the failure backoff must not delay a successful refresh.
func TestRun_ShortNonRenewableTTLNeverLapsesWithDefaultBackoff(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
		require.NoError(t, err)
		m := &timedMethod{}
		a := auth.NewAuthenticator(client, m, auth.WithLogger(slog.New(slog.DiscardHandler)))

		runFor(t, a, 10*time.Second)

		logins := m.Logins()
		require.GreaterOrEqual(t, len(logins), 10)
		for i := 1; i < len(logins); i++ {
			require.Less(t, logins[i].Sub(logins[i-1]), time.Second,
				"login %d started after the previous token expired", i)
		}
	})
}

// A lease that the login latency leaves too short to refresh ahead of expiry
// is refreshed on a bounded cadence and reported once.
func TestRun_UnusablyShortTTLWarnsOnceAndStaysBounded(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
		require.NoError(t, err)
		var logs bytes.Buffer
		m := &timedMethod{latency: 900 * time.Millisecond}
		a := auth.NewAuthenticator(client, m,
			auth.WithLogger(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn}))))

		runFor(t, a, 10*time.Second)

		logins := m.Logins()
		require.GreaterOrEqual(t, len(logins), 9)
		require.LessOrEqual(t, len(logins), 11, "one login per second of latency plus the minimum interval")
		require.Equal(t, 1, strings.Count(logs.String(), "too short to refresh"))
	})
}
