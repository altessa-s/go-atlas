// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package hmacsign

import (
	"crypto/hmac"
	"fmt"
	"time"
)

// Signer produces the signature header for an outbound request body under a
// [Scheme] and a secret. It is safe for concurrent use.
type Signer struct {
	scheme Scheme
	secret []byte
	opts   *options
}

// NewSigner builds a Signer that signs with secret using scheme. The scheme and
// secret are required; secret must be non-empty for the signature to carry any
// security.
func NewSigner(scheme Scheme, secret []byte, opts ...Option) *Signer {
	return &Signer{scheme: scheme, secret: secret, opts: newOptions(opts...)}
}

// HeaderName is the HTTP header the produced signature belongs in, a convenience
// forwarding to the scheme so callers need not hold it separately.
func (s *Signer) HeaderName() string { return s.scheme.HeaderName() }

// Sign returns the signature header value for body. For a timestamped scheme it
// stamps the current time (from the configured clock) into the signature.
func (s *Signer) Sign(body []byte) string {
	now := s.opts.clock()
	return s.scheme.FormatHeader(now, digest(s.secret, s.scheme.Message(now, body)))
}

// Verifier authenticates an inbound request body against the signature header a
// sender produced under the same [Scheme] and a shared secret. It is safe for
// concurrent use.
type Verifier struct {
	scheme  Scheme
	secrets [][]byte
	opts    *options
}

// NewVerifier builds a Verifier for scheme accepting secret. Additional secrets
// for rotation are supplied with [WithSecrets]; the timestamp tolerance (for a
// timestamped scheme) with [WithTolerance].
func NewVerifier(scheme Scheme, secret []byte, opts ...Option) *Verifier {
	o := newOptions(opts...)
	secrets := make([][]byte, 0, 1+len(o.secrets))
	secrets = append(secrets, secret)
	secrets = append(secrets, o.secrets...)
	return &Verifier{scheme: scheme, secrets: secrets, opts: o}
}

// HeaderName is the HTTP header the signature is read from, forwarding to the
// scheme.
func (v *Verifier) HeaderName() string { return v.scheme.HeaderName() }

// Verify authenticates body against the signature header. It returns nil when a
// configured secret produces a digest matching the header (compared in constant
// time). It returns [ErrMalformedSignature] for an unparsable header,
// [ErrTimestampOutOfTolerance] when a timestamped scheme's timestamp falls
// outside the tolerance window, and [ErrSignatureMismatch] when the header is
// well-formed but authentic to no configured secret. Pass the exact bytes that
// were signed.
func (v *Verifier) Verify(header string, body []byte) error {
	signedAt, sigs, err := v.scheme.ParseHeader(header)
	if err != nil {
		return err
	}
	if !signedAt.IsZero() && v.opts.tolerance > 0 {
		if err := v.checkTimestamp(signedAt); err != nil {
			return err
		}
	}
	msg := v.scheme.Message(signedAt, body)
	for _, secret := range v.secrets {
		expected := digest(secret, msg)
		for _, sig := range sigs {
			if hmac.Equal(expected, sig) {
				return nil
			}
		}
	}
	return ErrSignatureMismatch
}

// checkTimestamp rejects a signed timestamp further than the tolerance from now,
// in either direction (a future timestamp is as suspect as a stale one).
func (v *Verifier) checkTimestamp(ts time.Time) error {
	diff := v.opts.clock().Sub(ts)
	if diff < 0 {
		diff = -diff
	}
	if diff > v.opts.tolerance {
		return fmt.Errorf("%w: %s old", ErrTimestampOutOfTolerance, diff)
	}
	return nil
}
