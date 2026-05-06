// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package proxydial

import (
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFromURL_NilReturnsNilDialer(t *testing.T) {
	t.Parallel()

	dial, err := FromURL(nil)
	require.NoError(t, err)
	require.Nil(t, dial, "nil URL means direct dial; caller skips wiring")
}

func TestFromURL_HTTPSchemes(t *testing.T) {
	t.Parallel()

	for _, scheme := range []string{"http", "https"} {
		t.Run(scheme, func(t *testing.T) {
			t.Parallel()

			u, err := url.Parse(scheme + "://proxy.example.com:8443")
			require.NoError(t, err)

			dial, err := FromURL(u)
			require.NoError(t, err)
			require.NotNil(t, dial, "http/https scheme must produce a dialer")
		})
	}
}

func TestFromURL_SOCKSSchemes(t *testing.T) {
	t.Parallel()

	for _, scheme := range []string{"socks5", "socks5h"} {
		t.Run(scheme, func(t *testing.T) {
			t.Parallel()

			u, err := url.Parse(scheme + "://socks.example.com:1080")
			require.NoError(t, err)

			dial, err := FromURL(u)
			require.NoError(t, err)
			require.NotNil(t, dial, "socks5 scheme must produce a dialer")
		})
	}
}

func TestFromURL_RejectsUnknownScheme(t *testing.T) {
	t.Parallel()

	u, err := url.Parse("ftp://proxy.example.com:21")
	require.NoError(t, err)

	dial, err := FromURL(u)
	require.Error(t, err, "ftp scheme must surface at construction time")
	require.Nil(t, dial)
	require.Contains(t, err.Error(), "ftp")
}

func TestFromURL_OptionsApply(t *testing.T) {
	t.Parallel()

	u, err := url.Parse("https://proxy.example.com:8443")
	require.NoError(t, err)

	customDialer := &net.Dialer{Timeout: 1 * time.Millisecond}

	o := &options{dialer: DefaultDialer()}
	WithDialer(customDialer)(o)
	require.Same(t, customDialer, o.dialer, "WithDialer must override the default dialer")

	WithDialer(nil)(o)
	require.Same(t, customDialer, o.dialer, "nil arg to WithDialer is a no-op")

	dial, err := FromURL(u, WithDialer(customDialer))
	require.NoError(t, err)
	require.NotNil(t, dial)
}
