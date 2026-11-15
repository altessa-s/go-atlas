// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/security/vault/auth"

	vaultApi "github.com/hashicorp/vault/api"
)

// renewableMethod issues a renewable token with the given lease and records
// when each login started.
type renewableMethod struct {
	leaseSeconds int
	mu           sync.Mutex
	logins       []time.Time
}

func (m *renewableMethod) Authenticate(_ context.Context, c *vaultApi.Client) (*vaultApi.Secret, error) {
	m.mu.Lock()
	m.logins = append(m.logins, time.Now())
	m.mu.Unlock()
	c.SetToken("t")
	return &vaultApi.Secret{Auth: &vaultApi.SecretAuth{ClientToken: "t", Renewable: true, LeaseDuration: m.leaseSeconds}}, nil
}

func (*renewableMethod) Shutdown() error   { return nil }
func (*renewableMethod) Name() string      { return "renewable" }
func (*renewableMethod) IssuesToken() bool { return true }

func (m *renewableMethod) gaps() []time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	gaps := make([]time.Duration, 0, len(m.logins))
	for i := 1; i < len(m.logins); i++ {
		gaps = append(gaps, m.logins[i].Sub(m.logins[i-1]))
	}
	return gaps
}

// renewClient is a Vault client whose renew-self answers with renew(), usable
// inside a synctest bubble.
func renewClient(t *testing.T, renew func() (status int, body string)) *vaultApi.Client {
	t.Helper()
	transport := testhelpers.RoundTripFunc(func(req *http.Request) (*http.Response, error) {
		status, body := http.StatusNotFound, `{"errors":["not found"]}`
		if req.URL.Path == "/v1/auth/token/renew-self" {
			status, body = renew()
		}
		return &http.Response{
			StatusCode: status,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})
	client, err := vaultApi.NewClient(&vaultApi.Config{
		Address: "http://vault.test", MaxRetries: 0, HttpClient: &http.Client{Transport: transport},
	})
	require.NoError(t, err)
	return client
}

// renewedLease is a renew-self response that extends the token by seconds.
func renewedLease(seconds string) string {
	return `{"auth":{"client_token":"t","renewable":true,"lease_duration":` + seconds + `}}`
}

// A renewable token whose renewal cycle ends right after every login (the
// renewal grants no lease) must not make Run log in again in a tight loop:
// the logins are spaced by the growing failure backoff, and a cycle that then
// holds its token resets the spacing.
func TestRun_ShortRenewalCyclesBackOff(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var healthy atomic.Bool
		client := renewClient(t, func() (int, string) {
			if healthy.Load() {
				return http.StatusOK, renewedLease("3600")
			}
			return http.StatusOK, renewedLease("0")
		})
		m := &renewableMethod{leaseSeconds: 3600}
		a := auth.NewAuthenticator(client, m, auth.WithLogger(slog.New(slog.DiscardHandler)))

		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- a.Run(ctx) }()

		time.Sleep(5 * time.Minute) // fake time: only short cycles so far
		synctest.Wait()
		short := m.gaps()

		healthy.Store(true)
		time.Sleep(4 * time.Hour) // fake time: renewals now hold the token
		cancel()
		require.NoError(t, <-done)

		require.GreaterOrEqual(t, len(short), 6)
		require.LessOrEqual(t, len(short), 20, "logins must not spin")
		for n, gap := range short[:6] {
			floor := min(auth.DefaultBackoffBase<<n, auth.DefaultBackoffMax)
			require.GreaterOrEqual(t, gap, floor, "short cycle %d", n)
			require.LessOrEqual(t, gap, 2*floor+time.Second, "short cycle %d", n)
		}
		require.GreaterOrEqual(t, short[len(short)-1], auth.DefaultBackoffMax)

		require.LessOrEqual(t, len(m.gaps()), len(short)+2, "a healthy cycle keeps renewing instead of logging in")
	})
}
