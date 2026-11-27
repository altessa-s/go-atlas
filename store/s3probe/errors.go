// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package s3probe

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"

	httpclient "github.com/altessa-s/go-atlas-net/http/client"
)

// ErrInvalidRequest is returned when the request cannot be sent: the endpoint,
// bucket or keys are missing or malformed. Nothing was sent.
var ErrInvalidRequest = errors.New("s3probe: invalid request")

// Sentinels matched by errors.Is against a [TransportError] of the
// corresponding [Kind].
var (
	// ErrPolicyRefused matches a destination refused by the egress policy
	// before any connection was made.
	ErrPolicyRefused = errors.New("s3probe: destination refused by egress policy")
	// ErrUnreachable matches a name that did not resolve, a refused
	// connection or any other failure to reach the endpoint.
	ErrUnreachable = errors.New("s3probe: endpoint unreachable")
	// ErrTLS matches a failed TLS handshake or certificate verification.
	ErrTLS = errors.New("s3probe: TLS failure")
	// ErrTimeout matches an exhausted time limit.
	ErrTimeout = errors.New("s3probe: timeout")
)

// Kind classifies a transport failure.
type Kind int

// Transport failure kinds.
const (
	// KindUnreachable is a failure to reach the endpoint.
	KindUnreachable Kind = iota
	// KindPolicy is a destination refused by the egress policy before connecting.
	KindPolicy
	// KindTLS is a TLS handshake or certificate failure.
	KindTLS
	// KindTimeout is an exhausted time limit.
	KindTimeout
)

// String returns the lower-case name of the kind.
func (k Kind) String() string {
	switch k {
	case KindUnreachable:
		return "unreachable"
	case KindPolicy:
		return "policy"
	case KindTLS:
		return "tls"
	case KindTimeout:
		return "timeout"
	default:
		return "unknown"
	}
}

// TransportError reports that no HTTP response was received.
type TransportError struct {
	// Kind tells why the request got no response.
	Kind Kind
	// Err is the underlying error.
	Err error
}

// Error implements error.
func (e *TransportError) Error() string {
	return "s3probe: " + e.Kind.String() + ": " + e.Err.Error()
}

// Unwrap returns the underlying error.
func (e *TransportError) Unwrap() error { return e.Err }

// Is matches the sentinel of the error's kind.
func (e *TransportError) Is(target error) bool {
	switch e.Kind {
	case KindPolicy:
		return target == ErrPolicyRefused
	case KindTLS:
		return target == ErrTLS
	case KindTimeout:
		return target == ErrTimeout
	default:
		return target == ErrUnreachable
	}
}

// classify turns an error of the HTTP client into a [TransportError]. A
// canceled context is returned unchanged: the caller gave up, the endpoint did
// not fail.
func classify(err error) error {
	if errors.Is(err, context.Canceled) {
		return err
	}

	return &TransportError{Kind: kindOf(err), Err: err}
}

func kindOf(err error) Kind {
	if errors.Is(err, httpclient.ErrSSRFBlocked) {
		return KindPolicy
	}

	if isTLS(err) {
		return KindTLS
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return KindTimeout
	}

	if netErr, ok := errors.AsType[net.Error](err); ok && netErr.Timeout() {
		return KindTimeout
	}

	return KindUnreachable
}

func isTLS(err error) bool {
	if _, ok := errors.AsType[*tls.CertificateVerificationError](err); ok {
		return true
	}

	if _, ok := errors.AsType[x509.UnknownAuthorityError](err); ok {
		return true
	}

	if _, ok := errors.AsType[x509.HostnameError](err); ok {
		return true
	}

	if _, ok := errors.AsType[x509.CertificateInvalidError](err); ok {
		return true
	}

	if _, ok := errors.AsType[tls.RecordHeaderError](err); ok {
		return true
	}

	_, ok := errors.AsType[*tls.AlertError](err)

	return ok
}
