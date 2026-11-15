// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPreResponseErrorAllowlist(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"nil", nil},
		{"canceled", context.Canceled},
		{"deadline", context.DeadlineExceeded},
		{"dial", &net.OpError{Op: "dial", Err: &os.SyscallError{Syscall: "connect", Err: syscall.ECONNREFUSED}}},
		{"dial child", &net.OpError{Op: "dial", Err: errors.New("connection establishment failed")}},
		{"DNS", &net.DNSError{Err: "no such host", Name: "idp.invalid"}},
		{"TLS verification", &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}},
		{"certificate invalid", x509.CertificateInvalidError{Reason: x509.Expired}},
		{"hostname", x509.HostnameError{Certificate: &x509.Certificate{}, Host: "idp.invalid"}},
		{"system roots", x509.SystemRootsError{}},
		{"TLS alert", tls.AlertError(40)},
		{"TLS record", tls.RecordHeaderError{Msg: "invalid record", RecordHeader: [5]byte{'s', 'e', 'c', 'r', 't'}}},
		{"reset", syscall.ECONNRESET},
		{"host unreachable", syscall.EHOSTUNREACH},
		{"network unreachable", syscall.ENETUNREACH},
		{"timeout", syscall.ETIMEDOUT},
		{"guard", ErrInsecureEndpoint},
		{"redirect guard", ErrRedirectNotAllowed},
		{"endpoint guard", &insecureEndpointError{message: "endpoint must be an absolute https URL"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.err == nil {
				require.NoError(t, preResponseError(tc.err))
				return
			}
			for _, cause := range []error{tc.err, &url.Error{Op: "Post", URL: "https://idp.invalid/token", Err: tc.err}} {
				got := preResponseError(cause)
				require.Equal(t, cause.Error(), got.Error())
				require.ErrorIs(t, got, cause)
			}
		})
	}
}

// White-box coverage pins the fail-closed classifier without relying on private net/http parser types.
func TestPreResponseErrorRejectsUnknownTreeNodes(t *testing.T) {
	t.Parallel()
	const secret = "private-parser-secret"
	unknown := errors.New(secret)
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"unknown", unknown},
		{"URL", &url.Error{Op: "Post", URL: "https://idp.invalid/token", Err: unknown}},
		{"proxyconnect", &net.OpError{Op: "proxyconnect", Err: unknown}},
		{"proxyconnect dial", &net.OpError{Op: "proxyconnect", Err: &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}}},
		{"read", &net.OpError{Op: "read", Err: syscall.ECONNRESET}},
		{"write", &net.OpError{Op: "write", Err: syscall.ECONNRESET}},
		{"DNS child", &net.DNSError{Err: secret, UnwrapErr: unknown}},
		{"TLS verification child", &tls.CertificateVerificationError{Err: unknown}},
		{"system roots child", x509.SystemRootsError{Err: unknown}},
		{"syscall child", &os.SyscallError{Syscall: "connect", Err: unknown}},
		{"wrapped cancellation", fmt.Errorf("%s: %w", secret, context.Canceled)},
		{"joined cancellation", errors.Join(context.Canceled, unknown)},
		{"joined guard", errors.Join(ErrInsecureEndpoint, unknown)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := preResponseError(tc.err)
			require.Equal(t, "request failed before a response was received", got.Error())
			require.Equal(t, tc.err, errors.Unwrap(got))
			require.ErrorIs(t, got, tc.err)
			require.Equal(t, got.Error(), preResponseError(got).Error())
		})
	}
}
