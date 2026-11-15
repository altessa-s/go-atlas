// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	egressrule "github.com/altessa-s/go-atlas-net/egressrule"
	httpclient "github.com/altessa-s/go-atlas-net/http/client"
	httpconfig "github.com/altessa-s/go-atlas/config/http"
	factory "github.com/altessa-s/go-atlas/factory/ext/net/httpclient"
)

func TestHTTPClientSSRF_ClientOptions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		cfg      *httpconfig.ClientSSRF
		wantOpts int
		wantErr  bool
	}{
		{
			name:     "nil receiver keeps protected default",
			cfg:      nil,
			wantOpts: 0,
		},
		{
			name:     "zero value keeps protected default",
			cfg:      &httpconfig.ClientSSRF{},
			wantOpts: 0,
		},
		{
			name:     "disabled emits opt-out option",
			cfg:      &httpconfig.ClientSSRF{Disabled: true},
			wantOpts: 1,
		},
		{
			name:     "disabled ignores allowed cidrs",
			cfg:      &httpconfig.ClientSSRF{Disabled: true, AllowedCIDRs: []string{"10.0.0.0/8"}},
			wantOpts: 1,
		},
		{
			name:     "allowed cidrs emit exemption option",
			cfg:      &httpconfig.ClientSSRF{AllowedCIDRs: []string{"10.0.0.0/8", "192.168.0.0/16"}},
			wantOpts: 1,
		},
		{
			name:    "invalid cidr surfaces an error",
			cfg:     &httpconfig.ClientSSRF{AllowedCIDRs: []string{"not-a-cidr"}},
			wantErr: true,
		},
		{
			name:     "rules emit an egress policy",
			cfg:      &httpconfig.ClientSSRF{Deny: []egressrule.Rule{{Host: "*.internal"}}},
			wantOpts: 1,
		},
		{
			name: "rules and allowed cidrs",
			cfg: &httpconfig.ClientSSRF{
				Allow:        []egressrule.Rule{{CIDR: "10.0.0.0/8"}},
				AllowedCIDRs: []string{"192.168.0.0/16"},
			},
			wantOpts: 2,
		},
		{
			name:     "disabled ignores rules",
			cfg:      &httpconfig.ClientSSRF{Disabled: true, Deny: []egressrule.Rule{{}}},
			wantOpts: 1,
		},
		{
			name:    "rejected rule surfaces an error",
			cfg:     &httpconfig.ClientSSRF{Allow: []egressrule.Rule{{Host: "a.example"}}},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts, err := factory.SSRFOptions(tc.cfg)
			if tc.wantErr {
				require.Error(t, err)
				require.Nil(t, opts)
				return
			}
			require.NoError(t, err)
			require.Len(t, opts, tc.wantOpts)
		})
	}
}

func TestSSRFOptions_InvalidRuleWrapsEngineError(t *testing.T) {
	t.Parallel()

	_, err := factory.SSRFOptions(&httpconfig.ClientSSRF{Deny: []egressrule.Rule{{Port: 22}, {Host: "http://a"}}})
	require.ErrorIs(t, err, httpclient.ErrInvalidEgressRule)
	require.ErrorContains(t, err, "deny[1].host")
}

// TestSSRFOptions_EgressPolicyEndToEnd installs a configured policy in a real
// client: the loopback test server is reachable through an allow rule (or an
// allowed CIDR, which is an allow rule of the policy) until a deny rule
// matches it, and the refusal names the configured rule.
func TestSSRFOptions_EgressPolicyEndToEnd(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	_, portText, err := net.SplitHostPort(u.Host)
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)

	cases := []struct {
		name     string
		cfg      *httpconfig.ClientSSRF
		wantRule string // empty: the request succeeds
	}{
		{
			name:     "private default without rules exempting loopback",
			cfg:      &httpconfig.ClientSSRF{Deny: []egressrule.Rule{{Host: "*.internal"}}},
			wantRule: "-",
		},
		{
			name: "allow rule admits loopback",
			cfg:  &httpconfig.ClientSSRF{Allow: []egressrule.Rule{{CIDR: "127.0.0.0/8"}}},
		},
		{
			name: "allowed cidr is an allow rule of the policy",
			cfg:  &httpconfig.ClientSSRF{Deny: []egressrule.Rule{{Port: 22}}, AllowedCIDRs: []string{"127.0.0.0/8"}},
		},
		{
			name: "deny wins over allow",
			cfg: &httpconfig.ClientSSRF{
				Allow: []egressrule.Rule{{CIDR: "127.0.0.0/8"}},
				Deny:  []egressrule.Rule{{Name: "blockTestServer", CIDR: "127.0.0.1/32", Port: port}},
			},
			wantRule: "blockTestServer cidr=127.0.0.1/32 port=" + portText,
		},
		{
			name: "deny wins over allowed cidrs",
			cfg: &httpconfig.ClientSSRF{
				AllowedCIDRs: []string{"127.0.0.0/8"},
				Deny:         []egressrule.Rule{{CIDR: "127.0.0.0/8"}},
			},
			wantRule: "deny[0] cidr=127.0.0.0/8",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts, err := factory.SSRFOptions(tc.cfg)
			require.NoError(t, err)
			c := httpclient.New(append(opts, httpclient.WithRetryMax(0), httpclient.WithoutProxy())...)
			t.Cleanup(c.CloseIdleConnections)

			resp, err := c.Get(srv.URL)
			if tc.wantRule == "" {
				require.NoError(t, err)
				require.NoError(t, resp.Body.Close())
				return
			}
			require.ErrorIs(t, err, httpclient.ErrSSRFBlocked)
			if tc.wantRule != "-" {
				ssrfErr := httpclient.IsSSRFError(err)
				require.NotNil(t, ssrfErr)
				require.Equal(t, tc.wantRule, ssrfErr.Rule)
			}
		})
	}
}
