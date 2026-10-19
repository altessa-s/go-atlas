// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"net"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/types/redacted"
	"github.com/altessa-s/go-atlas/transport/proxydial/factory"

	proxyconfig "github.com/altessa-s/go-atlas/config/proxy"
)

// ambiguousSecret is the credential fragment that an unencoded "/" in the
// password moves into the URL host.
const ambiguousSecret = "s3cr3t-fragment"

var ambiguousProxyURLs = map[string]string{
	"without port": "http://svc:prefix@" + ambiguousSecret + "/remaining@proxy:3128",
	"with port":    "http://svc:prefix@" + ambiguousSecret + ":1/remaining@proxy:3128",
	"socks5":       "socks5://svc:prefix@" + ambiguousSecret + ":1/remaining@proxy:1080",
}

// TestBuild_RejectsAmbiguousUserInfo asserts that the dialer builder and the
// client-option factories reject a proxy URL whose credentials spill into the
// host, with an error that quotes no credential fragment, and that nothing is
// resolved or dialed.
func TestBuild_RejectsAmbiguousUserInfo(t *testing.T) {
	t.Parallel()

	for name, raw := range ambiguousProxyURLs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var lookups, dials atomic.Int32
			dialer := &net.Dialer{
				Resolver: &net.Resolver{PreferGo: true, Dial: func(context.Context, string, string) (net.Conn, error) {
					lookups.Add(1)
					return nil, net.ErrClosed
				}},
				Control: func(string, string, syscall.RawConn) error {
					dials.Add(1)
					return net.ErrClosed
				},
			}
			cfg := &proxyconfig.Config{Mode: proxyconfig.ModeURL, URL: redacted.URL(raw)}

			dial, err := factory.New(cfg).UseDialer(dialer).Build()
			require.Error(t, err)
			require.Nil(t, dial)
			require.NotContains(t, err.Error(), ambiguousSecret)

			httpOpts, err := factory.HTTPClientOptions(cfg)
			require.Error(t, err)
			require.Nil(t, httpOpts)
			require.NotContains(t, err.Error(), ambiguousSecret)

			grpcOpts, err := factory.GRPCClientOptions(cfg)
			require.Error(t, err)
			require.Nil(t, grpcOpts)
			require.NotContains(t, err.Error(), ambiguousSecret)

			require.Zero(t, lookups.Load(), "no DNS lookup of the fragment host")
			require.Zero(t, dials.Load(), "no dial to the fragment host")
		})
	}
}
