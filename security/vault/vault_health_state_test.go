// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package vault_test

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/security/vault"
	"github.com/altessa-s/go-atlas/security/vault/auth"
	"github.com/altessa-s/go-atlas/security/vault/auth/token"

	vaultApi "github.com/hashicorp/vault/api"
)

// lockedBuffer is a log sink read by the test while the renewal run writes to it.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// A static token that never expires needs no renewal: the run ends, and the
// client stays healthy.
func TestCheckHealth_StaticNonExpiringTokenServes(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		fake := &fakeVault{}
		v, err := vault.New(t.Context(), vault.WithVaultClient(fake.client(t)), vault.WithAuthMethod(token.New("supplied-token")))
		require.NoError(t, err)
		require.Equal(t, health.StatusNotServing, v.CheckHealth(t.Context()), "renewal never started")

		require.NoError(t, v.RunRenewalWithContext(t.Context()))
		time.Sleep(24 * time.Hour) // fake time: the run has long ended
		synctest.Wait()

		require.NoError(t, v.RenewalErr())
		require.Equal(t, health.StatusServing, v.CheckHealth(t.Context()))
		require.Equal(t, int32(1), fake.lookups.Load())

		require.NoError(t, v.Close())
		require.Equal(t, health.StatusNotServing, v.CheckHealth(t.Context()), "a closed client never serves")
		require.ErrorIs(t, v.RunRenewalWithContext(t.Context()), vault.ErrClosed)
	})
}

// Without an auth method the caller manages the token: a client that carries
// one is healthy, one that does not is not.
func TestCheckHealth_NoAuthMethod(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		client := (&fakeVault{}).client(t)
		v, err := vault.New(t.Context(), vault.WithVaultClient(client))
		require.NoError(t, err)

		require.Equal(t, health.StatusNotServing, v.CheckHealth(t.Context()), "no token on the client")

		client.SetToken("preset-token")
		require.Equal(t, health.StatusServing, v.CheckHealth(t.Context()))

		require.NoError(t, v.Close())
		require.Equal(t, "preset-token", client.Token(), "a supplied token is left on the client")
		require.Equal(t, health.StatusNotServing, v.CheckHealth(t.Context()))
	})
}

// The client is not healthy until the first login completes, nor after a
// start that failed.
func TestCheckHealth_BeforeFirstLoginAndAfterFailedStart(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		fake := &fakeVault{}
		method := &scriptedLogin{
			script:  []func(*vaultApi.Client) (*vaultApi.Secret, error){rejectLogin, issue(0)},
			release: make(chan struct{}),
		}
		v, err := vault.New(t.Context(), vault.WithVaultClient(fake.client(t)), vault.WithAuthMethod(method))
		require.NoError(t, err)
		t.Cleanup(func() { _ = v.Close() })

		started := make(chan error, 1)
		go func() { started <- v.RunRenewalWithContext(t.Context()) }()
		synctest.Wait()
		require.Equal(t, health.StatusNotServing, v.CheckHealth(t.Context()), "the first login is still in flight")

		close(method.release)
		require.ErrorIs(t, <-started, auth.ErrInvalidCredentials)
		synctest.Wait()
		require.Equal(t, health.StatusNotServing, v.CheckHealth(t.Context()), "the start failed")
		require.ErrorIs(t, v.RenewalErr(), auth.ErrInvalidCredentials)

		require.NoError(t, v.RunRenewalWithContext(t.Context()))
		require.Equal(t, health.StatusServing, v.CheckHealth(t.Context()))
	})
}

// A static non-renewable token with a finite TTL cannot be replaced by the
// token method: the run must not poll lookup-self, must announce the expiry
// once, and must surface it through RenewalErr and CheckHealth when it comes.
func TestRenewal_StaticFiniteTokenIsNotPolledAndExpiryIsReported(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		fake := &fakeVault{lookupBody: `{"data":{"renewable":false,"ttl":3600}}`}
		var logs lockedBuffer
		v, err := vault.New(t.Context(),
			vault.WithVaultClient(fake.client(t)),
			vault.WithAuthMethod(token.New("batch-token")),
			vault.WithLogger(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn}))),
		)
		require.NoError(t, err)
		t.Cleanup(func() { _ = v.Close() })

		require.NoError(t, v.RunRenewalWithContext(t.Context()))
		time.Sleep(59 * time.Minute) // fake time
		synctest.Wait()

		require.Equal(t, int32(1), fake.lookups.Load(), "the token must not be looked up again")
		require.NoError(t, v.RenewalErr())
		require.Equal(t, health.StatusServing, v.CheckHealth(t.Context()))
		require.Equal(t, 1, strings.Count(logs.String(), "cannot obtain a new one, it will expire"))

		time.Sleep(2 * time.Minute) // fake time: past the TTL
		synctest.Wait()

		require.Equal(t, int32(1), fake.lookups.Load())
		require.ErrorIs(t, v.RenewalErr(), auth.ErrTokenExpired)
		require.Equal(t, health.StatusNotServing, v.CheckHealth(t.Context()))
	})
}
