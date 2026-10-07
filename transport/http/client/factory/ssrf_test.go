// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/client/factory"

	httpconfig "github.com/altessa-s/go-atlas/config/http"
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
