// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestProxy_Validate_ValidCases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  Proxy
	}{
		{"empty_passthrough", Proxy{}},
		{"none", Proxy{Mode: ProxyModeNone}},
		{"url_http", Proxy{Mode: ProxyModeURL, URL: "http://proxy:3128"}},
		{"url_https", Proxy{Mode: ProxyModeURL, URL: "https://proxy:3128"}},
		{"url_socks5", Proxy{Mode: ProxyModeURL, URL: "socks5://proxy:1080"}},
		{"url_socks5h", Proxy{Mode: ProxyModeURL, URL: "socks5h://proxy:1080"}},
		{"host_no_auth", Proxy{Mode: ProxyModeHost, Host: "proxy", Port: 3128}},
		{"host_with_auth", Proxy{
			Mode: ProxyModeHost, Host: "proxy", Port: 3128,
			Auth: &ProxyAuth{Username: "svc", Password: "secret"},
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
		cfg  Proxy
	}{
		{"unknown_mode", Proxy{Mode: "bogus"}},
		{"env_mode_no_longer_valid", Proxy{Mode: "env"}},
		{"url_mode_missing_url", Proxy{Mode: ProxyModeURL}},
		{"url_mode_invalid_url", Proxy{Mode: ProxyModeURL, URL: "::not-a-url"}},
		{"url_mode_no_port", Proxy{Mode: ProxyModeURL, URL: "http://proxy.corp"}},
		{"host_mode_missing_host", Proxy{Mode: ProxyModeHost, Port: 3128}},
		{"host_mode_missing_port", Proxy{Mode: ProxyModeHost, Host: "proxy"}},
		{"host_mode_port_zero", Proxy{Mode: ProxyModeHost, Host: "proxy", Port: 0}},
		{"host_mode_port_negative", Proxy{Mode: ProxyModeHost, Host: "proxy", Port: -1}},
		{"host_mode_port_too_large", Proxy{Mode: ProxyModeHost, Host: "proxy", Port: 70000}},
		{"url_mode_with_host_field", Proxy{Mode: ProxyModeURL, URL: "http://p:1", Host: "x"}},
		{"empty_mode_with_url_field", Proxy{URL: "http://p:1"}},
		{"none_mode_with_host_field", Proxy{Mode: ProxyModeNone, Host: "x"}},
		{"host_mode_auth_without_username", Proxy{
			Mode: ProxyModeHost, Host: "p", Port: 1,
			Auth: &ProxyAuth{Password: "x"},
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
	var p *Proxy
	require.NoError(t, p.Validate())
}

func TestProxy_PasswordRedaction(t *testing.T) {
	t.Parallel()

	password := "topsecret-do-not-leak"
	auth := &ProxyAuth{Username: "svc", Password: Secret(password)}

	// Cover every common output sink: fmt %+v on the dereferenced struct
	// (the pointer-print path shows an address, not the fields), and the
	// fmt.Stringer/%v path on RedactedString directly.
	rendered := fmt.Sprintf("%+v", *auth) + " | " + fmt.Sprintf("%v", auth.Password)
	require.NotContains(t, rendered, password)
	require.Contains(t, rendered, "<redacted>")
}

func TestDefaultProxy(t *testing.T) {
	t.Parallel()

	got := DefaultProxy()
	require.Empty(t, string(got.Mode), "default Mode is the empty zero value (passthrough)")
	require.NoError(t, got.Validate())

}
