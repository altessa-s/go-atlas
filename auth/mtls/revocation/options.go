// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package revocation

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"net/http"
	"time"
)

// FailMode controls what a [Checker] returns when the revocation status of a
// certificate cannot be determined — the OCSP responder is unreachable, the
// certificate carries no responder URL, the responder answers "unknown", or no
// configured issuer matches.
type FailMode int

const (
	// FailOpen accepts the certificate on an indeterminate result. It maximizes
	// availability — a responder outage does not break new mTLS connections — at
	// the cost of briefly accepting a peer whose revocation could not be checked.
	// This is the default.
	FailOpen FailMode = iota
	// FailClosed rejects the certificate unless the responder confirms it is
	// good. Use it when serving a possibly-revoked peer is less acceptable than a
	// temporary outage.
	FailClosed
)

// Defaults for the network/cache tunables.
const (
	// DefaultFailMode is the availability-preserving default ([FailOpen]).
	DefaultFailMode = FailOpen
	// DefaultTimeout bounds a single revocation check (all OCSP attempts).
	DefaultTimeout = 5 * time.Second
	// DefaultMaxAttempts is the OCSP request attempt count (with backoff).
	DefaultMaxAttempts = 3
	// DefaultMaxTTL caps how long a cached status is trusted, regardless of the
	// responder's NextUpdate.
	DefaultMaxTTL = time.Hour
)

type options struct {
	httpClient  *http.Client     `optgen:"notnil"`
	failMode    FailMode         `optgen:"manual,default=DefaultFailMode"`
	timeout     time.Duration    `optgen:"default=DefaultTimeout"`
	maxAttempts int              `optgen:"default=DefaultMaxAttempts"`
	maxTTL      time.Duration    `optgen:"default=DefaultMaxTTL"`
	now         func() time.Time `opt:"-"`
}

// WithFailMode selects how the checker reacts to an indeterminate status.
// Unknown values leave the default ([DefaultFailMode]) in place.
func WithFailMode(mode FailMode) Option {
	return func(o *options) {
		switch mode {
		case FailOpen, FailClosed:
			o.failMode = mode
		}
	}
}

// WithClock overrides the time source (cache expiry, OCSP NextUpdate math). It
// is intended for tests; production uses [time.Now].
func WithClock(now func() time.Time) Option {
	return func(o *options) {
		if now != nil {
			o.now = now
		}
	}
}
