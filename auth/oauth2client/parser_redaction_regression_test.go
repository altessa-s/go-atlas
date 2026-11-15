// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/oauth2client"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func malformedResponseEndpoint(t testing.TB, response string) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	done := make(chan struct{})
	failures := make(chan error, 1)
	t.Cleanup(func() {
		_ = listener.Close()
		<-done
		select {
		case err := <-failures:
			require.NoError(t, err)
		default:
		}
	})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			err = conn.SetDeadline(time.Now().Add(5 * time.Second))
			if err == nil {
				var req *http.Request
				req, err = http.ReadRequest(bufio.NewReader(conn))
				if err == nil {
					_, err = io.Copy(io.Discard, req.Body)
					_ = req.Body.Close()
				}
			}
			if err == nil {
				_, err = io.WriteString(conn, response)
			}
			_ = conn.Close()
			if err != nil {
				failures <- err
				return
			}
		}
	}()
	return "http://" + listener.Addr().String() + "/token"
}

func TestMalformedHTTPResponseRedaction(t *testing.T) {
	t.Parallel()
	const secret = "private-client-secret"
	for _, tc := range []struct{ name, response string }{
		{"content length", "HTTP/1.1 200 OK\r\nContent-Length: " + secret + "\r\n\r\n"},
		{"status line", secret + "\r\n\r\n"},
		{"header line", "HTTP/1.1 200 OK\r\n" + secret + "\r\n\r\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, flow := range []string{"native", "form", "retry disabled", "retry exhausted", "unreplayable"} {
				t.Run(flow, func(t *testing.T) {
					t.Parallel()
					endpoint := malformedResponseEndpoint(t, tc.response)
					base := &http.Transport{}
					t.Cleanup(base.CloseIdleConnections)
					var logs bytes.Buffer
					var err error
					if flow == "native" || flow == "form" {
						err = requestBoundaryError(t, flow, endpoint, &http.Client{Transport: base}, &logs)
					} else {
						attempts := 1
						if flow == "retry disabled" {
							attempts = 0
						}
						transport := oauth2client.RetryTransport(base, oauth2client.WithTransportAttempts(attempts),
							oauth2client.WithTransportBackoff(time.Millisecond, time.Millisecond))
						req, requestErr := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewBufferString("form"))
						require.NoError(t, requestErr)
						if flow == "unreplayable" {
							req.GetBody = nil
						}
						resp, transportErr := transport.RoundTrip(req)
						if resp != nil {
							_ = resp.Body.Close()
						}
						err = transportErr
						slog.New(slog.NewJSONHandler(&logs, nil)).ErrorContext(t.Context(), "request failed", "error", err)
					}
					require.Error(t, err)
					require.NotContains(t, err.Error(), secret)
					require.NotEmpty(t, logs.String())
					require.NotContains(t, logs.String(), secret)
					var original error
					cause := errors.Unwrap(err)
					var requestError *url.Error
					if errors.As(err, &requestError) {
						cause = requestError
					}
					for ; cause != nil; cause = errors.Unwrap(cause) {
						if bytes.Contains([]byte(cause.Error()), []byte(secret)) {
							original = cause
						}
					}
					require.NotNil(t, original)
					require.ErrorIs(t, err, original)
				})
			}
		})
	}
}

func TestProxyConnectResponseRedaction(t *testing.T) {
	t.Parallel()
	const secret = "private-proxy-secret"
	for _, flow := range []string{"native", "form"} {
		t.Run(flow, func(t *testing.T) {
			t.Parallel()
			endpoint := malformedResponseEndpoint(t, "HTTP/1.1 407 "+secret+"\r\nContent-Length: 0\r\n\r\n")
			proxy, err := url.Parse(endpoint)
			require.NoError(t, err)
			base := &http.Transport{Proxy: http.ProxyURL(proxy)}
			t.Cleanup(base.CloseIdleConnections)
			var logs bytes.Buffer
			err = requestBoundaryError(t, flow, "https://idp.invalid/token", &http.Client{Transport: base}, &logs)
			require.Error(t, err)
			require.NotContains(t, err.Error(), secret)
			require.NotEmpty(t, logs.String())
			require.NotContains(t, logs.String(), secret)
		})
	}
}

func TestPreResponseConnectionErrorsRetainText(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"connection refused", "dial timeout", "DNS", "TLS", "cancellation"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			for _, flow := range []string{"native", "form", "retry disabled", "retry exhausted"} {
				t.Run(flow, func(t *testing.T) {
					t.Parallel()
					base := &http.Transport{}
					t.Cleanup(base.CloseIdleConnections)
					endpoint := "https://idp.invalid/token"
					ctx := t.Context()
					var target error
					switch kind {
					case "connection refused":
						listener, err := net.Listen("tcp", "127.0.0.1:0")
						require.NoError(t, err)
						endpoint = "http://" + listener.Addr().String() + "/token"
						require.NoError(t, listener.Close())
						target = syscall.ECONNREFUSED
					case "dial timeout":
						endpoint = "https://192.0.2.1:443/token"
						// Expire before connecting so the test does not depend on routing or firewall behavior.
						dialer := &net.Dialer{Timeout: time.Nanosecond}
						base.DialContext = dialer.DialContext
						target = context.DeadlineExceeded
					case "DNS":
						dialer := &net.Dialer{Resolver: &net.Resolver{
							PreferGo: true,
							Dial: func(context.Context, string, string) (net.Conn, error) {
								return nil, &net.DNSError{Err: "no such host", Name: "idp.invalid", IsNotFound: true}
							},
						}}
						base.DialContext = dialer.DialContext
					case "TLS":
						server := httptest.NewUnstartedServer(http.NotFoundHandler())
						server.Config.ErrorLog = log.New(io.Discard, "", 0)
						server.StartTLS()
						t.Cleanup(server.Close)
						endpoint = server.URL
						base.TLSClientConfig = &tls.Config{RootCAs: x509.NewCertPool(), MinVersion: tls.VersionTLS12}
					case "cancellation":
						canceled, cancel := context.WithCancel(ctx)
						cancel()
						ctx = canceled
						target = context.Canceled
					}
					var original error
					transport := testhelpers.RoundTripFunc(func(req *http.Request) (*http.Response, error) {
						resp, err := base.RoundTrip(req.WithContext(ctx))
						original = err
						return resp, err
					})
					var logs bytes.Buffer
					var err error
					if flow == "native" || flow == "form" {
						err = requestBoundaryError(t, flow, endpoint, &http.Client{Transport: transport}, &logs)
					} else {
						attempts := 1
						if flow == "retry disabled" {
							attempts = 0
						}
						retrying := oauth2client.RetryTransport(transport, oauth2client.WithTransportAttempts(attempts),
							oauth2client.WithTransportBackoff(time.Millisecond, time.Millisecond))
						req, requestErr := http.NewRequestWithContext(t.Context(), http.MethodGet, endpoint, nil)
						require.NoError(t, requestErr)
						resp, transportErr := retrying.RoundTrip(req)
						if resp != nil {
							_ = resp.Body.Close()
						}
						err = transportErr
					}
					require.Error(t, original)
					require.ErrorIs(t, err, original)
					require.Contains(t, err.Error(), original.Error())
					if target != nil {
						require.ErrorIs(t, err, target)
					}
					if kind == "dial timeout" {
						require.Contains(t, err.Error(), "i/o timeout")
						var dial *net.OpError
						require.ErrorAs(t, err, &dial)
						require.Equal(t, "dial", dial.Op)
						require.True(t, dial.Timeout())
					}
					if kind == "DNS" {
						var dns *net.DNSError
						require.ErrorAs(t, err, &dns)
					}
					if kind == "TLS" {
						var certificate *tls.CertificateVerificationError
						require.ErrorAs(t, err, &certificate)
						var authority x509.UnknownAuthorityError
						require.ErrorAs(t, err, &authority)
					}
					if flow == "native" || flow == "form" {
						require.Contains(t, logs.String(), original.Error())
					}
				})
			}
		})
	}
}
