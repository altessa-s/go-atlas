// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSRFControl_BlocksLoopback(t *testing.T) {
	t.Parallel()
	ctrl := ssrfControl(nil)

	for _, addr := range []string{"127.0.0.1:80", "127.1.2.3:80", "[::1]:80", "[::ffff:127.0.0.1]:80", "[::ffff:10.0.0.1]:80"} {
		err := ctrl("tcp", addr, nil)
		require.ErrorIs(t, err, ErrSSRFBlocked, addr)
	}
}

func TestSSRFControl_AllowedCIDRsExemptLoopback(t *testing.T) {
	t.Parallel()
	ctrl := ssrfControl([]netip.Prefix{
		netip.MustParsePrefix("127.0.0.0/8"),
		netip.MustParsePrefix("::1/128"),
	})

	require.NoError(t, ctrl("tcp", "127.0.0.1:80", nil))
	require.NoError(t, ctrl("tcp", "[::1]:80", nil))
	require.NoError(t, ctrl("tcp", "[::ffff:127.0.0.1]:80", nil))
}

func TestSSRFControl_BlocksPrivateRFC1918(t *testing.T) {
	ctrl := ssrfControl(nil)

	tests := []struct {
		name    string
		address string
	}{
		{"10.x", "10.0.0.1:443"},
		{"172.16.x", "172.16.0.1:443"},
		{"192.168.x", "192.168.1.1:443"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ctrl("tcp", tt.address, nil)
			require.Error(t, err)

			var ssrfErr *SSRFError
			require.ErrorAs(t, err, &ssrfErr)
		})
	}
}

func TestSSRFControl_BlocksMetadata(t *testing.T) {
	ctrl := ssrfControl(nil)

	err := ctrl("tcp", "169.254.169.254:80", nil)
	require.Error(t, err)

	var ssrfErr *SSRFError
	require.ErrorAs(t, err, &ssrfErr)
	assert.Equal(t, "169.254.169.254", ssrfErr.IP)
}

func TestSSRFControl_AllowsPublicIP(t *testing.T) {
	ctrl := ssrfControl(nil)

	tests := []struct {
		name    string
		address string
	}{
		{"Google DNS", "8.8.8.8:443"},
		{"Cloudflare", "1.1.1.1:443"},
		{"Public web", "93.184.216.34:80"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ctrl("tcp", tt.address, nil)
			assert.NoError(t, err)
		})
	}
}

func TestSSRFControl_AllowedCIDRs(t *testing.T) {
	allowed := []netip.Prefix{
		netip.MustParsePrefix("10.0.1.0/24"),
	}
	ctrl := ssrfControl(allowed)

	// Allowed range should pass
	err := ctrl("tcp", "10.0.1.5:443", nil)
	assert.NoError(t, err)

	// Other private ranges should still be blocked
	err = ctrl("tcp", "10.0.2.5:443", nil)
	require.Error(t, err)

	var ssrfErr *SSRFError
	require.ErrorAs(t, err, &ssrfErr)
}

func TestSSRFControl_BlocksIPv6Private(t *testing.T) {
	ctrl := ssrfControl(nil)

	tests := []struct {
		name    string
		address string
	}{
		{"link-local", "[fe80::1]:80"},
		{"ULA", "[fd00::1]:80"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ctrl("tcp", tt.address, nil)
			require.Error(t, err)

			var ssrfErr *SSRFError
			require.ErrorAs(t, err, &ssrfErr)
		})
	}
}

func TestSSRFControl_Disabled(t *testing.T) {
	// SSRF protection is on by default; WithoutSSRFProtection turns it off, so
	// newCircuitBreakerClient does not wrap the transport.
	opts := *newOptions(WithoutSSRFProtection())
	assert.False(t, opts.ssrfProtection)
	assert.Nil(t, opts.ssrfAllowedCIDRs)
}

func TestSSRFControl_EnabledByDefault(t *testing.T) {
	opts := *newOptions()
	assert.True(t, opts.ssrfProtection, "SSRF protection must be on by default")
}

func TestSSRFError_ErrorMessage(t *testing.T) {
	err := &SSRFError{Host: "internal.local", IP: "10.0.0.1"}
	assert.Equal(t, "ssrf: connection to private/local address blocked: host=internal.local ip=10.0.0.1", err.Error())
}

func TestSSRFError_Is(t *testing.T) {
	err := &SSRFError{Host: "localhost", IP: "127.0.0.1"}
	assert.True(t, errors.Is(err, ErrSSRFBlocked))
	assert.False(t, errors.Is(err, ErrCircuitBreakerOpen))
}

func TestIsSSRFError(t *testing.T) {
	ssrfErr := &SSRFError{Host: "localhost", IP: "127.0.0.1"}
	wrapped := fmt.Errorf("dial failed: %w", ssrfErr)

	result := IsSSRFError(wrapped)
	require.NotNil(t, result)
	assert.Equal(t, "127.0.0.1", result.IP)

	assert.Nil(t, IsSSRFError(errors.New("some other error")))
	assert.Nil(t, IsSSRFError(nil))
}

func TestSSRFControl_InvalidAddress(t *testing.T) {
	ctrl := ssrfControl(nil)

	// Address without port should fail in SplitHostPort
	err := ctrl("tcp", "127.0.0.1", nil)
	require.Error(t, err)

	// Not an SSRFError — it's a parse error
	assert.Nil(t, IsSSRFError(err))
}

func TestNewSSRFSafeTransport(t *testing.T) {
	// Verify the transport is cloned and has a DialContext set
	base := &defaultTransport
	transport := newSSRFSafeTransport(base, nil)

	require.NotNil(t, transport)
	assert.NotNil(t, transport.DialContext)
	// Should be a different instance from base
	assert.NotSame(t, base, transport)
}

// defaultTransport is a baseline for testing.
var defaultTransport = *defaultHTTPTransport()

func defaultHTTPTransport() *http.Transport {
	return http.DefaultTransport.(*http.Transport)
}

// TestSSRFControlSignature verifies the control function matches the
// net.Dialer.Control signature.
func TestSSRFControlSignature(t *testing.T) {
	ctrl := ssrfControl(nil)

	// Verify it satisfies the expected function signature
	var fn func(network, address string, c syscall.RawConn) error = ctrl
	assert.NotNil(t, fn)
}

func TestSSRFControlBlocksZonedAddresses(t *testing.T) {
	t.Parallel()
	for _, addr := range []string{"[fe80::1%en0]:80", "[::1%lo0]:80", "[fd00::1%en0]:80"} {
		require.ErrorIs(t, ssrfControl(nil)("tcp6", addr, nil), ErrSSRFBlocked, addr)
	}
	require.NoError(t, ssrfControl([]netip.Prefix{netip.MustParsePrefix("fe80::/10")})("tcp6", "[fe80::1%en0]:80", nil))
}
