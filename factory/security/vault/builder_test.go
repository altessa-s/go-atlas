// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/types/redacted"
	"github.com/altessa-s/go-atlas/factory/security/vault"
	"github.com/altessa-s/go-atlas/security/vault"

	vaultconfig "github.com/altessa-s/go-atlas/config/vault"
)

// fakeVault answers the token lookup and the login endpoints with a
// non-renewable token and records the path and token of every request.
type fakeVault struct {
	mu       sync.Mutex
	requests []fakeRequest
}

type fakeRequest struct {
	path  string
	token string
}

func (f *fakeVault) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, fakeRequest{path: r.URL.Path, token: r.Header.Get("X-Vault-Token")})
	f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/v1/auth/token/lookup-self" {
		_, _ = w.Write([]byte(`{"data":{"renewable":false,"ttl":3600}}`))
		return
	}
	_, _ = w.Write([]byte(`{"auth":{"client_token":"issued","renewable":false,"lease_duration":3600}}`))
}

func (f *fakeVault) first(tb testing.TB) fakeRequest {
	tb.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	require.NotEmpty(tb, f.requests, "vault received no request")
	return f.requests[0]
}

// authenticate builds a Vault from b and runs one authentication against
// the fake server, so the test observes which auth method the factory built.
func authenticate(t *testing.T, b *factory.VaultBuilder) *vault.Vault {
	t.Helper()
	v, err := b.Build(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.StopRenewal() })
	require.NoError(t, v.RunRenewalWithContext(t.Context()))
	return v
}

func TestBuild_AuthMethods(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		auth      *vaultconfig.Auth
		wantPath  string
		wantToken string
	}{
		{
			name:      "Token",
			auth:      &vaultconfig.Auth{Method: vaultconfig.AuthMethodToken, Token: new(redacted.RedactedString(" s.token "))},
			wantPath:  "/v1/auth/token/lookup-self",
			wantToken: "s.token",
		},
		{
			name: "AppRoleMountPath",
			auth: &vaultconfig.Auth{
				Method:  vaultconfig.AuthMethodAppRole,
				Approle: &vaultconfig.AuthAppRole{RoleId: "role", SecretId: "secret", MountPath: new("team-approle")},
			},
			wantPath: "/v1/auth/team-approle/login",
		},
		{
			name: "AppRoleDefaultMountPath",
			auth: &vaultconfig.Auth{
				Method:  vaultconfig.AuthMethodAppRole,
				Approle: &vaultconfig.AuthAppRole{RoleId: "role", SecretId: "secret"},
			},
			wantPath: "/v1/auth/approle/login",
		},
		{
			name: "UserPassMountPath",
			auth: &vaultconfig.Auth{
				Method:   vaultconfig.AuthMethodUserPass,
				Userpass: &vaultconfig.AuthUserPass{Username: "alice", Password: "pw", MountPath: new("team-userpass")},
			},
			wantPath: "/v1/auth/team-userpass/login/alice",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fake := &fakeVault{}
			srv := httptest.NewServer(fake)
			t.Cleanup(srv.Close)

			v := authenticate(t, factory.New(&vaultconfig.Config{Address: srv.URL, Auth: tc.auth}))

			require.Equal(t, srv.URL, v.RawClient().Address())
			got := fake.first(t)
			require.Equal(t, tc.wantPath, got.path)
			if tc.wantToken != "" {
				require.Equal(t, tc.wantToken, got.token)
			}
		})
	}
}

// TestBuild_UsesTLSConfig pins that UseTlsConfig reaches the Vault client:
// the test server's certificate is trusted only through that config.
func TestBuild_UsesTLSConfig(t *testing.T) {
	t.Parallel()

	fake := &fakeVault{}
	srv := httptest.NewTLSServer(fake)
	t.Cleanup(srv.Close)

	roots := x509.NewCertPool()
	roots.AddCert(srv.Certificate())

	cfg := &vaultconfig.Config{
		Address: srv.URL,
		Auth:    &vaultconfig.Auth{Method: vaultconfig.AuthMethodToken, Token: new(redacted.RedactedString("s.token"))},
	}
	authenticate(t, factory.New(cfg).UseTlsConfig(&tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}))

	require.Equal(t, "/v1/auth/token/lookup-self", fake.first(t).path)
}

func TestBuild_Errors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  *vaultconfig.Config
	}{
		{name: "NilConfig", cfg: nil},
		{name: "NilAuth", cfg: &vaultconfig.Config{Address: "http://127.0.0.1:8200"}},
		{name: "UnknownMethod", cfg: &vaultconfig.Config{Auth: &vaultconfig.Auth{Method: "kerberos"}}},
		{name: "TokenWithoutToken", cfg: &vaultconfig.Config{Auth: &vaultconfig.Auth{Method: vaultconfig.AuthMethodToken}}},
		{name: "AppRoleWithoutConfig", cfg: &vaultconfig.Config{Auth: &vaultconfig.Auth{Method: vaultconfig.AuthMethodAppRole}}},
		{name: "UserPassWithoutConfig", cfg: &vaultconfig.Config{Auth: &vaultconfig.Auth{Method: vaultconfig.AuthMethodUserPass}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v, err := factory.New(tc.cfg).Build(t.Context())
			require.Error(t, err)
			require.Nil(t, v)
		})
	}
}
