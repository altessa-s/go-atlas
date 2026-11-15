// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package vaultconfig_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/types/redacted"

	tlsconfig "github.com/altessa-s/go-atlas/config/tls"
	vaultconfig "github.com/altessa-s/go-atlas/config/vault"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

func fieldErrors(t *testing.T, err error) validation.Errors {
	t.Helper()
	verrs, ok := errors.AsType[validation.Errors](err)
	require.True(t, ok, "want validation.Errors, got %v", err)
	return verrs
}

func TestDefaults(t *testing.T) {
	t.Parallel()

	c := vaultconfig.Default()
	require.Equal(t, "http://localhost:8200", c.Address)
	require.Equal(t, vaultconfig.AuthMethodToken, vaultconfig.DefaultAuth().Method)

	require.Contains(t, fieldErrors(t, c.Validate()), "Auth", "auth is mandatory")

	auth := vaultconfig.DefaultAuth()
	auth.Token = new(redacted.RedactedString("s.token"))
	c.Auth = &auth
	require.NoError(t, c.Validate())
}

func TestConfig_Normalize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  vaultconfig.Config
		want string
	}{
		{"plain host gets http", vaultconfig.Config{Address: "vault:8200"}, "http://vault:8200"},
		{"tls host gets https", vaultconfig.Config{Address: "vault:8200", TLS: &tlsconfig.Client{}}, "https://vault:8200"},
		{"scheme kept", vaultconfig.Config{Address: "https://vault:8200"}, "https://vault:8200"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := tc.cfg
			c.Normalize()
			require.Equal(t, tc.want, c.Address)
		})
	}
}

func TestConfig_ValidateAddress(t *testing.T) {
	t.Parallel()

	auth := vaultconfig.Auth{Method: vaultconfig.AuthMethodToken, Token: new(redacted.RedactedString("t"))}
	c := vaultconfig.Config{Address: "::bad", Auth: &auth}
	require.Contains(t, fieldErrors(t, c.Validate()), "Address")
}

func TestAuth_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		auth  vaultconfig.Auth
		field string
	}{
		{"unknown method", vaultconfig.Auth{Method: "kubernetes"}, "Method"},
		{"token missing", vaultconfig.Auth{Method: vaultconfig.AuthMethodToken}, "Token"},
		{"approle missing", vaultconfig.Auth{Method: vaultconfig.AuthMethodAppRole}, "Approle"},
		{"userpass missing", vaultconfig.Auth{Method: vaultconfig.AuthMethodUserPass}, "Userpass"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Contains(t, fieldErrors(t, tc.auth.Validate()), tc.field)
		})
	}

	require.NoError(t, (&vaultconfig.Auth{
		Method: vaultconfig.AuthMethodAppRole, Approle: &vaultconfig.AuthAppRole{RoleId: "r", SecretId: "s"},
	}).Validate())
	require.NoError(t, (&vaultconfig.Auth{
		Method: vaultconfig.AuthMethodUserPass, Userpass: &vaultconfig.AuthUserPass{Username: "u", Password: "p"},
	}).Validate())
}
