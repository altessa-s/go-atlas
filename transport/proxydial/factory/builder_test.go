// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"crypto/tls"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/types/redacted"
	"github.com/altessa-s/go-atlas/transport/proxydial/factory"

	proxyconfig "github.com/altessa-s/go-atlas/config/proxy"
)

func TestBuild_NilBuilder(t *testing.T) {
	t.Parallel()
	var b *factory.DialerBuilder
	dial, err := b.Build()
	require.NoError(t, err)
	require.Nil(t, dial)
}

func TestBuild_NilConfig(t *testing.T) {
	t.Parallel()
	dial, err := factory.New(nil).Build()
	require.NoError(t, err)
	require.Nil(t, dial, "nil cfg means direct dial; caller skips wiring")
}

func TestBuild_PassthroughReturnsNil(t *testing.T) {
	t.Parallel()
	for name, cfg := range map[string]proxyconfig.Config{
		"empty_mode":    {},
		"explicit_none": {Mode: proxyconfig.ModeNone},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dial, err := factory.New(&cfg).Build()
			require.NoError(t, err)
			require.Nil(t, dial)
		})
	}
}

func TestBuild_URL(t *testing.T) {
	t.Parallel()
	for name, scheme := range map[string]string{
		"http":    "http",
		"https":   "https",
		"socks5":  "socks5",
		"socks5h": "socks5h",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cfg := &proxyconfig.Config{Mode: proxyconfig.ModeURL, URL: scheme + "://proxy.example.com:8443"}
			dial, err := factory.New(cfg).Build()
			require.NoError(t, err)
			require.NotNil(t, dial, "%s scheme must produce a dialer", scheme)
		})
	}
}

func TestBuild_Host(t *testing.T) {
	t.Parallel()
	cfg := &proxyconfig.Config{Mode: proxyconfig.ModeHost, Host: "proxy.example.com", Port: 8443}
	dial, err := factory.New(cfg).Build()
	require.NoError(t, err)
	require.NotNil(t, dial)
}

func TestBuild_HostWithAuth(t *testing.T) {
	t.Parallel()
	cfg := &proxyconfig.Config{
		Mode: proxyconfig.ModeHost,
		Host: "proxy.example.com",
		Port: 8443,
		Auth: &proxyconfig.Auth{Username: "svc", Password: redacted.RedactedString("hunter2")},
	}
	dial, err := factory.New(cfg).Build()
	require.NoError(t, err)
	require.NotNil(t, dial)
}

func TestBuild_UnknownMode(t *testing.T) {
	t.Parallel()
	cfg := &proxyconfig.Config{Mode: "bogus"}
	dial, err := factory.New(cfg).Build()
	require.Error(t, err)
	require.Nil(t, dial)
}

func TestBuild_URLParseError(t *testing.T) {
	t.Parallel()
	cfg := &proxyconfig.Config{Mode: proxyconfig.ModeURL, URL: "::bad"}
	dial, err := factory.New(cfg).Build()
	require.Error(t, err)
	require.Nil(t, dial)
}

func TestBuild_FluentDependenciesApply(t *testing.T) {
	t.Parallel()
	cfg := &proxyconfig.Config{Mode: proxyconfig.ModeURL, URL: "https://proxy.example.com:8443"}
	dial, err := factory.New(cfg).
		UseDialer(&net.Dialer{Timeout: 1 * time.Millisecond}).
		UseProxyTLSConfig(&tls.Config{MinVersion: tls.VersionTLS12}).
		Build()
	require.NoError(t, err)
	require.NotNil(t, dial, "fluent dependencies must not break dialer construction")
}
