// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package proxyconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

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

// TestProxy_URLCredentialsNeverLeak asserts that a credential-bearing URL,
// valid or malformed, never shows in Validate errors nor in the fmt, slog,
// JSON and YAML renderings used to print the config, while Expose keeps the
// URL usable for the transport.
func TestProxy_URLCredentialsNeverLeak(t *testing.T) {
	t.Parallel()

	const secret = "pr0xy-s3cr3t"
	cases := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid", "http://svc:" + secret + "@proxy:3128", false},
		{"valid_socks5", "socks5://svc:" + secret + "@127.0.0.1:1080", false},
		{"malformed", "http://svc:" + secret + "@proxy example:3128/%zz", true},
		{"malformed_escape", "http://" + secret + "%zz", true},
		{"no_port", "http://svc:" + secret + "@proxy", true},
		{"no_scheme", "svc:" + secret + "@proxy:3128", true},
		{"bad_scheme", "ftp://svc:" + secret + "@proxy:21", true},
		{"slash_in_password", "http://svc:prefix@" + secret + "/remaining@proxy:3128", true},
		{"slash_in_password_with_port", "http://svc:prefix@" + secret + ":1/remaining@proxy:3128", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := Config{Mode: ModeURL, URL: redacted.URL(tc.url)}
			require.Equal(t, tc.url, cfg.URL.Expose())

			err := cfg.Validate()
			if tc.wantErr {
				require.Error(t, err)
				require.NotContains(t, err.Error(), secret, "Validate error leaks the URL")
			} else {
				require.NoError(t, err)
			}

			jsonOut, jerr := json.Marshal(cfg)
			require.NoError(t, jerr)
			yamlOut, yerr := yaml.Marshal(cfg)
			require.NoError(t, yerr)
			var logged bytes.Buffer
			slog.New(slog.NewJSONHandler(&logged, nil)).Info("config", slog.Any("proxy", cfg))
			var text bytes.Buffer
			slog.New(slog.NewTextHandler(&text, nil)).Info("config", slog.Any("proxy", &cfg))

			for name, out := range map[string]string{
				"fmt %v":    fmt.Sprintf("%v", cfg),
				"fmt %+v":   fmt.Sprintf("%+v", cfg),
				"fmt %#v":   fmt.Sprintf("%#v", cfg),
				"fmt ptr":   fmt.Sprintf("%+v", &cfg),
				"fmt %s":    fmt.Sprintf("%s", cfg.URL),
				"json":      string(jsonOut),
				"yaml":      string(yamlOut),
				"slog json": logged.String(),
				"slog text": text.String(),
			} {
				require.NotContains(t, out, secret, "%s output leaks the URL password", name)
			}
		})
	}
}

// TestProxy_ValidateErrorsOmitURLParts asserts that Validate errors quote no
// part of the URL: neither the scheme nor the host, where a mistyped password
// can end up.
func TestProxy_ValidateErrorsOmitURLParts(t *testing.T) {
	t.Parallel()

	const secret = "pr0xy-s3cr3t"
	for _, raw := range []string{
		secret + "://proxy:3128",
		"http://" + secret,
		"http://svc:prefix@" + secret + "/remaining@proxy:3128",
		"http://svc:prefix@" + secret + ":1/remaining@proxy:3128",
		"socks5://svc:prefix@" + secret + ":1/remaining@proxy:1080",
	} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()

			err := (&Config{Mode: ModeURL, URL: redacted.URL(raw)}).Validate()
			require.Error(t, err)
			require.NotContains(t, err.Error(), secret)
		})
	}
}

func TestProxy_URLUnmarshalFromYAML(t *testing.T) {
	t.Parallel()

	raw := "http://svc:secret@proxy:3128"
	var cfg Config
	require.NoError(t, yaml.Unmarshal([]byte("mode: url\nurl: "+raw+"\n"), &cfg))
	require.Equal(t, raw, cfg.URL.Expose())
	require.NoError(t, cfg.Validate())
}

func TestDefaultProxy(t *testing.T) {
	t.Parallel()

	got := Default()
	require.Empty(t, string(got.Mode), "default Mode is the empty zero value (passthrough)")
	require.NoError(t, got.Validate())

}
