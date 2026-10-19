// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlsconfig_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	tlsconfig "github.com/altessa-s/go-atlas/config/tls"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Serial: t.Setenv modifies a process-wide env var; t.Parallel panics when Setenv is in use.
func TestTlsClient_Normalize_SkipVerify_without_env(t *testing.T) {
	t.Setenv(tlsconfig.EnvAllowInsecureTLS, "")

	c := &tlsconfig.Client{SkipVerify: true}
	c.Normalize()

	require.False(t, c.SkipVerify, "SkipVerify must be reset to false without "+tlsconfig.EnvAllowInsecureTLS)
}

// Serial: see TestTlsClient_Normalize_SkipVerify_without_env.
func TestTlsClient_Normalize_SkipVerify_with_env(t *testing.T) {
	t.Setenv(tlsconfig.EnvAllowInsecureTLS, "true")

	c := &tlsconfig.Client{SkipVerify: true}
	c.Normalize()

	require.True(t, c.SkipVerify, "SkipVerify must remain true when "+tlsconfig.EnvAllowInsecureTLS+" is set")
}

// Serial: see TestTlsClient_Normalize_SkipVerify_without_env.
func TestTlsClient_Normalize_SkipVerify_with_env_case_insensitive(t *testing.T) {
	t.Setenv(tlsconfig.EnvAllowInsecureTLS, "TRUE")

	c := &tlsconfig.Client{SkipVerify: true}
	c.Normalize()

	require.True(t, c.SkipVerify, "SkipVerify must remain true when "+tlsconfig.EnvAllowInsecureTLS+" is TRUE (case-insensitive)")
}

func TestTlsClient_Normalize_SkipVerify_false_unaffected(t *testing.T) {
	t.Parallel()

	c := &tlsconfig.Client{SkipVerify: false}
	c.Normalize()

	require.False(t, c.SkipVerify)
}

func TestTlsClient_Normalize_SkipVerifyMode_defaults_to_enforce(t *testing.T) {
	t.Parallel()

	c := &tlsconfig.Client{}
	c.Normalize()

	require.Equal(t, tlsconfig.SkipVerifyModeEnforce, c.SkipVerifyMode,
		"empty SkipVerifyMode must default to enforce to match the YAML loader")
}

func TestTlsClient_Normalize_SkipVerifyMode_preserved(t *testing.T) {
	t.Parallel()

	c := &tlsconfig.Client{SkipVerifyMode: tlsconfig.SkipVerifyModeDisabled}
	c.Normalize()

	require.Equal(t, tlsconfig.SkipVerifyModeDisabled, c.SkipVerifyMode,
		"an explicit SkipVerifyMode must not be overwritten")
}

func TestTlsClient_Normalize_ResolvesPaths(t *testing.T) {
	t.Parallel()

	wd, err := os.Getwd()
	require.NoError(t, err)

	missing := filepath.Join(t.TempDir(), "missing.pem")
	c := &tlsconfig.Client{
		Certificate: "certs/client.pem",
		PrivateKey:  missing,
		CACerts:     []string{"ca/root.pem", missing},
	}
	c.Normalize()

	require.Equal(t, filepath.Join(wd, "certs", "client.pem"), c.Certificate, "relative path is resolved, not cleared")
	require.Equal(t, missing, c.PrivateKey, "absolute path to a missing file is kept")
	require.Equal(t, []string{filepath.Join(wd, "ca", "root.pem"), missing}, c.CACerts)
	verrs, ok := errors.AsType[validation.Errors](c.Validate())
	require.True(t, ok, "Validate must report the missing files")
	for _, field := range []string{"Certificate", "PrivateKey", "CACerts"} {
		require.Contains(t, verrs, field, "missing %s must be reported, not cleared", field)
	}
}

func TestTlsClient_Normalize_KeepsExistingRelativeCA(t *testing.T) {
	t.Parallel()

	// tls_client.go exists relative to the package directory the test runs in.
	c := &tlsconfig.Client{CACerts: []string{"tls_client.go"}}
	c.Normalize()

	wd, err := os.Getwd()
	require.NoError(t, err)
	require.Equal(t, []string{filepath.Join(wd, "tls_client.go")}, c.CACerts)

	verrs, _ := errors.AsType[validation.Errors](c.Validate())
	require.NotContains(t, verrs, "CACerts", "an existing relative CA must validate")
}

func TestTlsClientAuth_Normalize_ResolvesPaths(t *testing.T) {
	t.Parallel()

	wd, err := os.Getwd()
	require.NoError(t, err)

	c := &tlsconfig.ClientAuth{CACerts: []string{"ca/root.pem", "/abs/ca.pem", ""}}
	c.Normalize()

	require.Equal(t, []string{filepath.Join(wd, "ca", "root.pem"), "/abs/ca.pem", ""}, c.CACerts)
}

func TestTlsClient_Validate_EmptyPathsAreUnset(t *testing.T) {
	t.Parallel()

	// tls_client.go and doc.go exist in the package directory the test runs in.
	existing, err := filepath.Abs("tls_client.go")
	require.NoError(t, err)
	other, err := filepath.Abs("doc.go")
	require.NoError(t, err)
	missing := filepath.Join(t.TempDir(), "missing.pem")

	tests := []struct {
		name      string
		client    tlsconfig.Client
		badFields []string
	}{
		{"empty", tlsconfig.Client{}, nil},
		{"CA only", tlsconfig.Client{CACerts: []string{existing}}, nil},
		{"certificate pair", tlsconfig.Client{Certificate: existing, PrivateKey: other, CACerts: []string{existing}}, nil},
		{"missing certificate file", tlsconfig.Client{Certificate: missing, PrivateKey: other}, []string{"Certificate"}},
		{"missing key file", tlsconfig.Client{Certificate: existing, PrivateKey: missing}, []string{"PrivateKey"}},
		{"missing CA file", tlsconfig.Client{CACerts: []string{missing}}, []string{"CACerts"}},
		{"blank CA entry", tlsconfig.Client{CACerts: []string{""}}, []string{"CACerts"}},
		{"certificate without key", tlsconfig.Client{Certificate: existing}, []string{"PrivateKey"}},
		{"key without certificate", tlsconfig.Client{PrivateKey: other}, []string{"Certificate"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.client.Validate()
			if len(tc.badFields) == 0 {
				require.NoError(t, err)
				return
			}
			verrs, ok := errors.AsType[validation.Errors](err)
			require.True(t, ok, "Validate must return field errors: %v", err)
			require.Len(t, verrs, len(tc.badFields), "%v", verrs)
			for _, f := range tc.badFields {
				require.Contains(t, verrs, f)
			}
		})
	}
}
