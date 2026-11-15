// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package vault_test

import (
	"cmp"
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/security/vault"
	"github.com/altessa-s/go-atlas/security/vault/auth"
	"github.com/altessa-s/go-atlas/security/vault/auth/token"

	vaultApi "github.com/hashicorp/vault/api"
)

// fakeVault is an in-memory Vault HTTP API, usable inside a synctest bubble.
type fakeVault struct {
	// lookupGate, when set, makes token lookups wait for it or the request context.
	lookupGate chan struct{}
	// lookupBody is the lookup-self response; empty means a token that never expires.
	lookupBody   string
	lookups      atomic.Int32
	revoked      atomic.Int32
	lookupTokens sync.Map // X-Vault-Token of every lookup-self request
}

func (f *fakeVault) client(t *testing.T) *vaultApi.Client {
	t.Helper()
	respond := func(req *http.Request, status int, body string) *http.Response {
		return &http.Response{
			StatusCode: status,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}
	}
	transport := testhelpers.RoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/v1/sys/health":
			return respond(req, http.StatusOK, `{"initialized":true,"sealed":false}`), nil
		case "/v1/auth/token/revoke-self":
			f.revoked.Add(1)
			return respond(req, http.StatusNoContent, ""), nil
		case "/v1/auth/token/lookup-self":
			if f.lookupGate != nil {
				select {
				case <-f.lookupGate:
				case <-req.Context().Done():
					return nil, req.Context().Err()
				}
			}
			f.lookups.Add(1)
			f.lookupTokens.Store(req.Header.Get("X-Vault-Token"), struct{}{})
			return respond(req, http.StatusOK, cmp.Or(f.lookupBody, `{"data":{"renewable":false,"ttl":0}}`)), nil
		default:
			return respond(req, http.StatusNotFound, `{"errors":["not found"]}`), nil
		}
	})
	client, err := vaultApi.NewClient(&vaultApi.Config{
		Address: "http://vault.test", MaxRetries: 0, HttpClient: &http.Client{Transport: transport},
	})
	require.NoError(t, err)
	return client
}

// scriptedLogin answers each login from script; the last entry repeats. It
// mints its tokens.
type scriptedLogin struct {
	mu     sync.Mutex
	calls  int
	script []func(*vaultApi.Client) (*vaultApi.Secret, error)
	// release, when set, makes Authenticate wait for it, ignoring ctx.
	release  chan struct{}
	shutdown atomic.Bool
}

func (m *scriptedLogin) Authenticate(_ context.Context, c *vaultApi.Client) (*vaultApi.Secret, error) {
	if m.release != nil {
		<-m.release
	}
	m.mu.Lock()
	step := m.script[min(m.calls, len(m.script)-1)]
	m.calls++
	m.mu.Unlock()
	return step(c)
}
func (m *scriptedLogin) Shutdown() error { m.shutdown.Store(true); return nil }
func (*scriptedLogin) Name() string      { return "scripted" }
func (*scriptedLogin) IssuesToken() bool { return true }

func issue(ttlSeconds int) func(*vaultApi.Client) (*vaultApi.Secret, error) {
	return func(c *vaultApi.Client) (*vaultApi.Secret, error) {
		c.SetToken("issued-token")
		return &vaultApi.Secret{Auth: &vaultApi.SecretAuth{ClientToken: "issued-token", LeaseDuration: ttlSeconds}}, nil
	}
}

func rejectLogin(*vaultApi.Client) (*vaultApi.Secret, error) {
	return nil, auth.WrapAuthError("scripted", auth.ErrInvalidCredentials)
}

// A renewal run that ends on its own (here: the re-login is rejected) must not
// leave the client looking healthy, and a later successful start clears it.
func TestRenewal_UnexpectedEndIsReportedUntilRestart(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		fake := &fakeVault{}
		method := &scriptedLogin{script: []func(*vaultApi.Client) (*vaultApi.Secret, error){issue(1), rejectLogin}}
		v, err := vault.New(t.Context(), vault.WithVaultClient(fake.client(t)), vault.WithAuthMethod(method))
		require.NoError(t, err)

		require.NoError(t, v.RunRenewalWithContext(t.Context()))
		require.Equal(t, health.StatusServing, v.CheckHealth(t.Context()))
		require.NoError(t, v.RenewalErr())

		time.Sleep(2 * time.Second) // fake time: the one-second token is refreshed and the login is rejected
		synctest.Wait()

		require.ErrorIs(t, v.RenewalErr(), auth.ErrInvalidCredentials)
		require.Equal(t, health.StatusNotServing, v.CheckHealth(t.Context()))
		require.False(t, method.shutdown.Load(), "the method must stay usable for a restart")

		method.mu.Lock()
		method.script = []func(*vaultApi.Client) (*vaultApi.Secret, error){issue(0)}
		method.mu.Unlock()
		require.NoError(t, v.RunRenewalWithContext(t.Context()))
		require.NoError(t, v.RenewalErr())
		require.Equal(t, health.StatusServing, v.CheckHealth(t.Context()))

		require.NoError(t, v.Close())
		require.True(t, method.shutdown.Load(), "Close retires the method")
		require.Equal(t, int32(1), fake.revoked.Load())
	})
}

// A start that timed out must leave the auth method's credentials intact, so
// the next start can log in; only Close zeroes them.
func TestRenewal_RestartAfterTimeoutKeepsCredentials(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		fake := &fakeVault{lookupGate: make(chan struct{})}
		v, err := vault.New(t.Context(),
			vault.WithVaultClient(fake.client(t)),
			vault.WithAuthMethod(token.New("supplied-token")),
			vault.WithAuthTimeout(time.Second),
		)
		require.NoError(t, err)

		require.ErrorIs(t, v.RunRenewalWithContext(t.Context()), vault.ErrTimeout)
		require.Equal(t, health.StatusNotServing, v.CheckHealth(t.Context()))

		close(fake.lookupGate)
		require.NoError(t, v.RunRenewalWithContext(t.Context()))
		_, sent := fake.lookupTokens.Load("supplied-token")
		require.True(t, sent, "the restart must log in with the original token")
		require.Equal(t, health.StatusServing, v.CheckHealth(t.Context()))

		require.NoError(t, v.Close())
		require.ErrorIs(t, v.RunRenewalWithContext(t.Context()), vault.ErrClosed)
		require.Zero(t, fake.revoked.Load(), "a supplied token is never revoked")
	})
}

// A login that completes after the start already timed out still put a minted
// token on the client: Close must revoke it, and health must not report a
// token nobody renews.
func TestClose_RevokesTokenIssuedAfterStartTimedOut(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		fake := &fakeVault{}
		method := &scriptedLogin{
			script:  []func(*vaultApi.Client) (*vaultApi.Secret, error){issue(0)},
			release: make(chan struct{}),
		}
		v, err := vault.New(t.Context(),
			vault.WithVaultClient(fake.client(t)), vault.WithAuthMethod(method), vault.WithAuthTimeout(time.Second))
		require.NoError(t, err)

		require.ErrorIs(t, v.RunRenewalWithContext(t.Context()), vault.ErrTimeout)
		close(method.release)
		synctest.Wait()

		require.Equal(t, "issued-token", v.RawClient().Token())
		require.Equal(t, health.StatusNotServing, v.CheckHealth(t.Context()))

		require.NoError(t, v.Close())
		require.Equal(t, int32(1), fake.revoked.Load())
		require.Empty(t, v.RawClient().Token())
	})
}
