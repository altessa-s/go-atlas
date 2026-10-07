// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package proxyconfig

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/types/redacted"
)

func TestProxy_Validate_ValidCases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  Config
	}{
		{"empty_passthrough", Config{}},
		{"none", Config{Mode: ModeNone}},
		{"url_http", Config{Mode: ModeURL, URL: "http://proxy:3128"}},
		{"url_https", Config{Mode: ModeURL, URL: "https://proxy:3128"}},
		{"url_socks5", Config{Mode: ModeURL, URL: "socks5://proxy:1080"}},
		{"url_socks5h", Config{Mode: ModeURL, URL: "socks5h://proxy:1080"}},
		{"host_no_auth", Config{Mode: ModeHost, Host: "proxy", Port: 3128}},
		{"host_with_auth", Config{
			Mode: ModeHost, Host: "proxy", Port: 3128,
			Auth: &Auth{Username: "svc", Password: "secret"},
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.NoError(t, tc.cfg.Validate())
		})
	}
}

func TestProxy_Validate_InvalidCases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  Config
	}{
		{"unknown_mode", Config{Mode: "bogus"}},
		{"env_mode_no_longer_valid", Config{Mode: "env"}},
		{"url_mode_missing_url", Config{Mode: ModeURL}},
		{"url_mode_invalid_url", Config{Mode: ModeURL, URL: "::not-a-url"}},
		{"url_mode_no_port", Config{Mode: ModeURL, URL: "http://proxy.corp"}},
		{"host_mode_missing_host", Config{Mode: ModeHost, Port: 3128}},
		{"host_mode_missing_port", Config{Mode: ModeHost, Host: "proxy"}},
		{"host_mode_port_zero", Config{Mode: ModeHost, Host: "proxy", Port: 0}},
		{"host_mode_port_negative", Config{Mode: ModeHost, Host: "proxy", Port: -1}},
		{"host_mode_port_too_large", Config{Mode: ModeHost, Host: "proxy", Port: 70000}},
		{"url_mode_with_host_field", Config{Mode: ModeURL, URL: "http://p:1", Host: "x"}},
		{"empty_mode_with_url_field", Config{URL: "http://p:1"}},
		{"none_mode_with_host_field", Config{Mode: ModeNone, Host: "x"}},
		{"host_mode_auth_without_username", Config{
			Mode: ModeHost, Host: "p", Port: 1,
			Auth: &Auth{Password: "x"},
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Error(t, tc.cfg.Validate())
		})
	}
}

func TestProxy_Validate_NilReceiver(t *testing.T) {
	t.Parallel()
	var p *Config
	require.NoError(t, p.Validate())
}

func TestProxy_PasswordRedaction(t *testing.T) {
	t.Parallel()

	password := "topsecret-do-not-leak"
	auth := &Auth{Username: "svc", Password: redacted.RedactedString(password)}

	// Cover every common output sink: fmt %+v on the dereferenced struct
	// (the pointer-print path shows an address, not the fields), and the
	// fmt.Stringer/%v path on RedactedString directly.
	rendered := fmt.Sprintf("%+v", *auth) + " | " + fmt.Sprintf("%v", auth.Password)
	require.NotContains(t, rendered, password)
	require.Contains(t, rendered, "<redacted>")
}

func TestDefaultProxy(t *testing.T) {
	t.Parallel()

	got := Default()
	require.Empty(t, string(got.Mode), "default Mode is the empty zero value (passthrough)")
	require.NoError(t, got.Validate())

}
