// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGrpcProxy_Validate_ValidCases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  GrpcProxy
	}{
		{"empty_passthrough", GrpcProxy{}},
		{"none", GrpcProxy{Mode: GrpcProxyModeNone}},
		{"url_http", GrpcProxy{Mode: GrpcProxyModeURL, URL: "http://proxy:3128"}},
		{"url_https", GrpcProxy{Mode: GrpcProxyModeURL, URL: "https://proxy:3128"}},
		{"url_socks5", GrpcProxy{Mode: GrpcProxyModeURL, URL: "socks5://proxy:1080"}},
		{"url_socks5h", GrpcProxy{Mode: GrpcProxyModeURL, URL: "socks5h://proxy:1080"}},
		{"host_no_auth", GrpcProxy{Mode: GrpcProxyModeHost, Host: "proxy", Port: 3128}},
		{"host_with_auth", GrpcProxy{
			Mode: GrpcProxyModeHost, Host: "proxy", Port: 3128,
			Auth: &GrpcProxyAuth{Username: "svc", Password: "secret"},
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.NoError(t, tc.cfg.Validate())
		})
	}
}

func TestGrpcProxy_Validate_InvalidCases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  GrpcProxy
	}{
		{"unknown_mode", GrpcProxy{Mode: "bogus"}},
		{"env_mode_no_longer_valid", GrpcProxy{Mode: "env"}},
		{"url_mode_missing_url", GrpcProxy{Mode: GrpcProxyModeURL}},
		{"url_mode_invalid_url", GrpcProxy{Mode: GrpcProxyModeURL, URL: "::not-a-url"}},
		{"url_mode_no_port", GrpcProxy{Mode: GrpcProxyModeURL, URL: "http://proxy.corp"}},
		{"host_mode_missing_host", GrpcProxy{Mode: GrpcProxyModeHost, Port: 3128}},
		{"host_mode_missing_port", GrpcProxy{Mode: GrpcProxyModeHost, Host: "proxy"}},
		{"host_mode_port_zero", GrpcProxy{Mode: GrpcProxyModeHost, Host: "proxy", Port: 0}},
		{"host_mode_port_negative", GrpcProxy{Mode: GrpcProxyModeHost, Host: "proxy", Port: -1}},
		{"host_mode_port_too_large", GrpcProxy{Mode: GrpcProxyModeHost, Host: "proxy", Port: 70000}},
		{"url_mode_with_host_field", GrpcProxy{Mode: GrpcProxyModeURL, URL: "http://p:1", Host: "x"}},
		{"empty_mode_with_url_field", GrpcProxy{URL: "http://p:1"}},
		{"none_mode_with_host_field", GrpcProxy{Mode: GrpcProxyModeNone, Host: "x"}},
		{"host_mode_auth_without_username", GrpcProxy{
			Mode: GrpcProxyModeHost, Host: "p", Port: 1,
			Auth: &GrpcProxyAuth{Password: "x"},
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Error(t, tc.cfg.Validate())
		})
	}
}

func TestGrpcProxy_Validate_NilReceiver(t *testing.T) {
	t.Parallel()
	var p *GrpcProxy
	assert.NoError(t, p.Validate())
}

func TestGrpcProxy_ClientOptions_NilReceiver(t *testing.T) {
	t.Parallel()
	var p *GrpcProxy
	opts, err := p.ClientOptions()
	assert.NoError(t, err)
	assert.Nil(t, opts)
}

func TestGrpcProxy_ClientOptions_PassthroughReturnsNil(t *testing.T) {
	t.Parallel()

	// Empty Mode == "no override" — leave grpc-go's HTTPS_PROXY default
	// in place.
	cfg := GrpcProxy{}
	opts, err := cfg.ClientOptions()
	assert.NoError(t, err)
	assert.Nil(t, opts)
}

func TestGrpcProxy_ClientOptions_NonEnvReturnsOption(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  GrpcProxy
	}{
		{"none", GrpcProxy{Mode: GrpcProxyModeNone}},
		{"url", GrpcProxy{Mode: GrpcProxyModeURL, URL: "http://proxy:3128"}},
		{"host_no_auth", GrpcProxy{Mode: GrpcProxyModeHost, Host: "proxy", Port: 3128}},
		{"host_with_auth", GrpcProxy{
			Mode: GrpcProxyModeHost, Host: "proxy", Port: 3128,
			Auth: &GrpcProxyAuth{Username: "svc", Password: "secret"},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opts, err := tc.cfg.ClientOptions()
			assert.NoError(t, err)
			assert.Len(t, opts, 1)
		})
	}
}

func TestGrpcProxy_ClientOptions_UnknownMode(t *testing.T) {
	t.Parallel()
	cfg := GrpcProxy{Mode: "bogus"}
	opts, err := cfg.ClientOptions()
	assert.Error(t, err)
	assert.Nil(t, opts)
}

func TestGrpcProxy_ClientOptions_URLParseError(t *testing.T) {
	t.Parallel()
	cfg := GrpcProxy{Mode: GrpcProxyModeURL, URL: "::bad"}
	opts, err := cfg.ClientOptions()
	assert.Error(t, err)
	assert.Nil(t, opts)
}

func TestGrpcProxy_PasswordRedaction(t *testing.T) {
	t.Parallel()

	password := "topsecret-grpc-do-not-leak"
	auth := &GrpcProxyAuth{Username: "svc", Password: Secret(password)}

	rendered := fmt.Sprintf("%+v", *auth) + " | " + fmt.Sprintf("%v", auth.Password)
	assert.NotContains(t, rendered, password)
	assert.Contains(t, rendered, redacted)
}

func TestGrpcProxy_userinfo(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		auth *GrpcProxyAuth
		want *url.Userinfo
	}{
		{"nil_auth", nil, nil},
		{"empty_username", &GrpcProxyAuth{}, nil},
		{"username_only", &GrpcProxyAuth{Username: "svc"}, url.User("svc")},
		{"user_password", &GrpcProxyAuth{Username: "svc", Password: "p"}, url.UserPassword("svc", "p")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := &GrpcProxy{Auth: tc.auth}
			got := p.userinfo()
			if tc.want == nil {
				assert.Nil(t, got)
				return
			}
			assert.Equal(t, tc.want.String(), got.String())
		})
	}
}

func TestDefaultGrpcProxy(t *testing.T) {
	t.Parallel()

	got := DefaultGrpcProxy()
	assert.Empty(t, string(got.Mode), "default Mode is the empty zero value (passthrough)")
	assert.NoError(t, got.Validate())

	opts, err := got.ClientOptions()
	assert.NoError(t, err)
	assert.Nil(t, opts)
}
