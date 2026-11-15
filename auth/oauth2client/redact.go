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
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"syscall"

	"golang.org/x/oauth2"
)

// errorSummary shows the error code and lengths of withheld free-text fields.
// A response without a usable code is represented by its body length only.
func errorSummary(code, description, uri string, bodyLen int) string {
	switch code {
	case "invalid_request", "invalid_client", "invalid_grant", "unauthorized_client", "unsupported_grant_type", "invalid_scope",
		"authorization_pending", "slow_down", "access_denied", "expired_token", "server_error", "temporarily_unavailable":
	default:
		return fmt.Sprintf("response body of %d bytes", bodyLen)
	}
	summary := fmt.Sprintf("error %q", code)
	if description != "" {
		summary += fmt.Sprintf(" (description: %d bytes withheld)", len(description))
	}
	if uri != "" {
		summary += fmt.Sprintf(" (uri: %d bytes withheld)", len(uri))
	}
	return summary
}

// redactedError replaces untrusted response and decoder error text. The
// original error stays reachable through errors.Is and errors.As.
type redactedError struct {
	err  error
	text string
}

func (e *redactedError) Error() string { return e.text }
func (e *redactedError) Unwrap() error { return e.err }

// redactError returns err with the IdP response body removed from its text
// when it carries an [*oauth2.RetrieveError]. Decoder errors pass through
// responseError at the response boundary.
func redactError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := err.(*redactedError); ok {
		return err
	}
	re, ok := errors.AsType[*oauth2.RetrieveError](err)
	if !ok {
		if previous, ok := errors.AsType[*redactedError](err); ok {
			return &redactedError{err: err, text: previous.text}
		}
		return err
	}
	status := 0
	if re.Response != nil {
		status = re.Response.StatusCode
	}
	return responseError(err, status, len(re.Body))
}

// redacted wraps an x/oauth2 token source so its errors pass through
// [redactError].
func redacted(inner oauth2.TokenSource, response *responseMetadata) oauth2.TokenSource {
	return &redactingSource{inner: inner, response: response}
}

// redactingSource is the [oauth2.TokenSource] of [redacted].
type redactingSource struct {
	mu       sync.Mutex
	response *responseMetadata
	inner    oauth2.TokenSource
}

// Token returns a token from the wrapped source.
func (s *redactingSource) Token() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.response.received.Store(false)
	s.response.status.Store(0)
	s.response.length.Store(0)
	tok, err := s.inner.Token()
	if err != nil {
		return nil, s.response.redact(err)
	}
	return tok, nil
}

// responseError withholds untrusted decoder text while preserving its cause.
func responseError(err error, status, length int) error {
	if err == nil {
		return nil
	}
	re, received := errors.AsType[*oauth2.RetrieveError](err)
	summary := errorSummary("", "", "", length)
	if received {
		summary = errorSummary(re.ErrorCode, re.ErrorDescription, re.ErrorURI, length)
	}
	return &redactedError{err: err, text: fmt.Sprintf("identity provider returned status %d: %s", status, summary)}
}

type responseMetadata struct {
	received atomic.Bool
	status   atomic.Int64
	length   atomic.Int64
}

func (m *responseMetadata) redact(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errors.AsType[*oauth2.RetrieveError](err); ok {
		return redactError(err)
	}
	if m.received.Load() {
		return redactError(responseError(err, int(m.status.Load()), int(m.length.Load())))
	}
	return preResponseError(err)
}

func preResponseError(err error) error {
	if err == nil || responseFreeError(err) {
		return err
	}
	if redacted, ok := redactError(err).(*redactedError); ok {
		return redacted
	}
	return &redactedError{err: err, text: "request failed before a response was received"}
}

func responseFreeError(err error) bool {
	switch err {
	case nil, context.Canceled, context.DeadlineExceeded,
		syscall.ECONNREFUSED, syscall.ECONNRESET, syscall.EHOSTUNREACH, syscall.ENETUNREACH, syscall.ETIMEDOUT,
		ErrInsecureEndpoint, ErrRedirectNotAllowed, ErrTokenExchange, ErrSubjectTokenRequired, ErrNoTokenEndpoint,
		ErrTokenRequest, ErrRevocation, ErrDeviceAuth, ErrClientAssertion, ErrJWTBearerConfig:
		return true
	}
	switch e := err.(type) {
	case *url.Error:
		return responseFreeError(e.Err)
	case *net.OpError:
		// Dial failures precede any connection; other operations can follow peer bytes.
		return e.Op == "dial"
	case *net.DNSError:
		return responseFreeError(e.Unwrap())
	case *os.SyscallError:
		return responseFreeError(e.Err)
	case *tls.CertificateVerificationError:
		return responseFreeError(e.Err)
	case x509.SystemRootsError:
		return responseFreeError(e.Err)
	case x509.CertificateInvalidError, x509.HostnameError, x509.UnknownAuthorityError, tls.AlertError, tls.RecordHeaderError:
		return true
	case *insecureEndpointError:
		return true
	default:
		return false
	}
}

func withResponseClient(ctx context.Context, client *http.Client) (context.Context, *responseMetadata) {
	metadata := &responseMetadata{}
	clone := *client
	clone.Transport = &responseTransport{base: client.Transport, metadata: metadata}
	return withHTTPClient(ctx, &clone), metadata
}

type responseTransport struct {
	base     http.RoundTripper
	metadata *responseMetadata
}

func (t *responseTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Redirect hops carry the response that supplied their URL; retain its metadata.
	if req.Response == nil {
		t.metadata.received.Store(false)
		t.metadata.status.Store(0)
		t.metadata.length.Store(0)
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(req)
	if err == nil {
		t.metadata.received.Store(true)
		t.metadata.status.Store(int64(resp.StatusCode))
		t.metadata.length.Store(0)
		if resp.Body != nil {
			resp.Body = &responseBody{ReadCloser: resp.Body, metadata: t.metadata}
		}
	}
	return resp, err
}

type responseBody struct {
	io.ReadCloser
	metadata *responseMetadata
}

func (b *responseBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.metadata.length.Add(int64(n))
	return n, err
}
