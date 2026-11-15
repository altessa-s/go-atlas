// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package vault_test

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/security/vault"
	"github.com/altessa-s/go-atlas/security/vault/auth"

	vaultApi "github.com/hashicorp/vault/api"
)

// loginMethod installs a never-expiring token on the client. issues selects
// whether it declares itself an auth.TokenIssuer that mints the token.
type loginMethod struct {
	issues bool
}

func (m loginMethod) Authenticate(_ context.Context, c *vaultApi.Client) (*vaultApi.Secret, error) {
	c.SetToken("issued-token")
	return &vaultApi.Secret{Auth: &vaultApi.SecretAuth{ClientToken: "issued-token"}}, nil
}
func (loginMethod) Shutdown() error     { return nil }
func (loginMethod) Name() string        { return "login" }
func (m loginMethod) IssuesToken() bool { return m.issues }

var _ auth.TokenIssuer = loginMethod{}

// revokeServer counts revoke-self calls and answers them with status.
func revokeServer(t *testing.T, status int) (*vaultApi.Client, *atomic.Int32) {
	t.Helper()
	var revoked atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/auth/token/revoke-self" && r.Header.Get("X-Vault-Token") == "issued-token" {
			revoked.Add(1)
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	client, err := vaultApi.NewClient(&vaultApi.Config{Address: srv.URL, MaxRetries: 0})
	require.NoError(t, err)
	return client, &revoked
}

func TestClose_RevokesIssuedToken(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status int
	}{
		{"revocation succeeds", http.StatusNoContent},
		{"revocation failure is not returned", http.StatusInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client, revoked := revokeServer(t, tc.status)
			v, err := vault.New(t.Context(), vault.WithVaultClient(client), vault.WithAuthMethod(loginMethod{issues: true}))
			require.NoError(t, err)
			require.NoError(t, v.RunRenewalWithContext(t.Context()))

			require.NoError(t, v.Close())
			require.Equal(t, int32(1), revoked.Load())
			require.Empty(t, client.Token(), "the revoked token must be dropped from the client")

			require.NoError(t, v.Close())
			require.Equal(t, int32(1), revoked.Load(), "a second Close has nothing left to revoke")
		})
	}
}

func TestClose_KeepsSuppliedToken(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		method auth.Method
		run    bool
	}{
		{"method does not issue tokens", loginMethod{issues: false}, true},
		{"method without the TokenIssuer interface", &gatedMethod{}, true},
		{"renewal never ran", loginMethod{issues: true}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client, revoked := revokeServer(t, http.StatusNoContent)
			client.SetToken("issued-token")
			v, err := vault.New(t.Context(), vault.WithVaultClient(client), vault.WithAuthMethod(tc.method))
			require.NoError(t, err)
			if tc.run {
				require.NoError(t, v.RunRenewalWithContext(t.Context()))
			}

			require.NoError(t, v.Close())
			require.Zero(t, revoked.Load())
			require.Equal(t, "issued-token", client.Token())
		})
	}
}

func TestNew_TLSConfigWithSuppliedClientIsRejected(t *testing.T) {
	t.Parallel()
	client, err := vaultApi.NewClient(vaultApi.DefaultConfig())
	require.NoError(t, err)

	_, err = vault.New(t.Context(),
		vault.WithVaultClient(client),
		vault.WithTLSConfig(&tls.Config{MinVersion: tls.VersionTLS13}),
	)
	require.ErrorIs(t, err, vault.ErrTLSConfigNotApplied)
}
