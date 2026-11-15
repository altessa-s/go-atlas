// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package revocation

//go:generate go tool optgen generate

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
	// the cost of accepting a peer whose revocation could not be checked: anyone
	// able to block the responder can then present a revoked certificate. It is
	// an explicit opt-in through [WithFailMode].
	FailOpen FailMode = iota
	// FailClosed rejects the certificate unless the responder confirms it is
	// good. This is the default.
	FailClosed
)

// Defaults for the network/cache tunables.
const (
	// DefaultFailMode is the safe default ([FailClosed]): a certificate whose
	// revocation status cannot be confirmed is rejected.
	DefaultFailMode = FailClosed
	// DefaultTimeout bounds a single revocation check (all OCSP attempts).
	DefaultTimeout = 5 * time.Second
	// DefaultAttemptTimeout bounds one OCSP request. It is sized so that the
	// first request and [DefaultMaxAttempts] retries, with their backoff, fit
	// into [DefaultTimeout] even when every request stalls.
	DefaultAttemptTimeout = time.Second
	// DefaultMaxAttempts is the OCSP request attempt count (with backoff).
	DefaultMaxAttempts = 3
	// DefaultMaxTTL caps how long a cached status is trusted, regardless of the
	// responder's NextUpdate.
	DefaultMaxTTL = time.Hour
	// DefaultMaxCacheEntries bounds the in-memory status cache so a server
	// presenting many distinct client certificates cannot grow it without limit.
	DefaultMaxCacheEntries = 4096
	// DefaultClockSkew is the tolerance applied to an OCSP response's
	// thisUpdate and nextUpdate when checking that a "good" response is current.
	DefaultClockSkew = 5 * time.Minute
	// DefaultMaxResponseAge bounds how long after its thisUpdate an OCSP
	// response without a nextUpdate is trusted. Such a response never expires
	// on its own, so without this bound a captured "good" answer could be
	// replayed indefinitely. It also caps how long that status is cached.
	DefaultMaxResponseAge = 24 * time.Hour
)

type options struct {
	// WithHTTPClient replaces the HTTP client used for the OCSP POST. The default
	// client is bounded by the check timeout ([WithTimeout]), refuses a redirect
	// to a different host ([ErrCrossHostRedirect]) and refuses to connect to a
	// non-public address ([ErrPrivateResponder]); a client supplied here is used
	// as is, so its timeout, redirect policy and destination restrictions are
	// the caller's responsibility. The responder URL scheme and
	// [WithAllowedResponders] are still enforced. A nil client keeps the default.
	httpClient *http.Client `optgen:"notnil"`
	// WithAllowedResponders restricts the OCSP responders the checker contacts
	// to the listed hosts. An entry is a host name or IP literal ("ocsp.corp"),
	// optionally with a port ("ocsp.corp:8080", "[::1]:8080"); it is compared
	// case-insensitively with the host of the responder URL in the certificate.
	// A responder on another host is refused with [ErrResponderNotAllowed]
	// without a request. Naming a responder vouches for it, so the default
	// client's non-public address refusal ([ErrPrivateResponder]) is lifted
	// for the listed hosts. Entries are trimmed and lower-cased and empty ones
	// are dropped; a list that names no host is no allowlist, so the default
	// address refusal stays in force.
	allowedResponders []string `optval:"clone,lower,nonempty"`
	// WithInsecureAllowPrivateResponders lets the default client connect to a
	// responder on a loopback, private, link-local, unique-local or otherwise
	// non-public address. The responder URL comes from the peer's certificate,
	// so with this option anyone able to obtain a certificate with a chosen
	// authorityInfoAccess from a trusted CA can make the server send requests
	// to internal services. Prefer [WithAllowedResponders].
	insecureAllowPrivateResponders bool
	// WithFailMode selects how the checker reacts to an indeterminate status.
	// Unknown values leave the default ([DefaultFailMode]) in place.
	failMode FailMode      `opt:"FailMode" optcheck:"oneof=[FailOpen,FailClosed]" optgen:"default=DefaultFailMode,onfail=skip"`
	timeout  time.Duration `optgen:"default=DefaultTimeout"`
	// WithAttemptTimeout bounds one OCSP request, so a stalled responder costs
	// one attempt instead of the whole check and the retries still run. The
	// check as a whole stays bounded by [WithTimeout]; a value above it is
	// capped to it. Non-positive values keep the default
	// ([DefaultAttemptTimeout]).
	attemptTimeout  time.Duration `optgen:"default=DefaultAttemptTimeout"`
	maxAttempts     int           `optgen:"default=DefaultMaxAttempts"`
	maxTTL          time.Duration `optgen:"default=DefaultMaxTTL"`
	maxCacheEntries int           `optgen:"default=DefaultMaxCacheEntries"`
	clockSkew       time.Duration `optgen:"default=DefaultClockSkew" optval:"positive=allow_zero"`
	maxResponseAge  time.Duration `optgen:"default=DefaultMaxResponseAge"`
	// WithClock overrides the time source (cache expiry, OCSP NextUpdate math). It
	// is intended for tests; production uses [time.Now].
	now func() time.Time `opt:"Clock" optgen:"notnil"`
}
