// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHTTPClientSSRF_ClientOptions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		cfg      *HTTPClientSSRF
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
			cfg:      &HTTPClientSSRF{},
			wantOpts: 0,
		},
		{
			name:     "disabled emits opt-out option",
			cfg:      &HTTPClientSSRF{Disabled: true},
			wantOpts: 1,
		},
		{
			name:     "disabled ignores allowed cidrs",
			cfg:      &HTTPClientSSRF{Disabled: true, AllowedCIDRs: []string{"10.0.0.0/8"}},
			wantOpts: 1,
		},
		{
			name:     "allowed cidrs emit exemption option",
			cfg:      &HTTPClientSSRF{AllowedCIDRs: []string{"10.0.0.0/8", "192.168.0.0/16"}},
			wantOpts: 1,
		},
		{
			name:    "invalid cidr surfaces an error",
			cfg:     &HTTPClientSSRF{AllowedCIDRs: []string{"not-a-cidr"}},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts, err := tc.cfg.ClientOptions()
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

func TestHTTPClientSSRF_Validate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		cfg     *HTTPClientSSRF
		wantErr bool
	}{
		{name: "nil receiver", cfg: nil},
		{name: "no cidrs", cfg: &HTTPClientSSRF{}},
		{name: "valid cidrs", cfg: &HTTPClientSSRF{AllowedCIDRs: []string{"10.0.0.0/8", "fd00::/8"}}},
		{name: "invalid cidr", cfg: &HTTPClientSSRF{AllowedCIDRs: []string{"10.0.0.0/8", "bogus"}}, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.cfg.Validate()
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestDefaultHTTPClientSSRF_IsStrict(t *testing.T) {
	t.Parallel()

	cfg := DefaultHTTPClientSSRF()
	require.False(t, cfg.Disabled, "default must keep SSRF protection enabled")

	opts, err := cfg.ClientOptions()
	require.NoError(t, err)
	require.Empty(t, opts, "default relies on httpclient.New's protected default, so emits no options")
}
