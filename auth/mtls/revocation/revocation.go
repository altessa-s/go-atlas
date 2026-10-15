// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package revocation

import (
	"bytes"
	"context"
	"crypto"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas/core/retry"

	"golang.org/x/crypto/ocsp"
	"golang.org/x/sync/singleflight"

	coremtls "github.com/altessa-s/go-atlas/auth/mtls"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

const (
	ocspContentType  = "application/ocsp-request"
	maxOCSPRespBytes = 1 << 20 // 1 MiB ceiling on an OCSP response body.

	// OCSP retry backoff parameters.
	retryBaseDelay = 50 * time.Millisecond
	retryMaxDelay  = time.Second
	retryJitter    = 0.2
)

var (
	// ErrNoIssuer indicates no configured issuer matches the certificate, so an
	// OCSP request cannot be built.
	ErrNoIssuer = errors.New("revocation: no configured issuer matches the certificate")
	// ErrNoResponder indicates the certificate carries no OCSP responder URL.
	ErrNoResponder = errors.New("revocation: certificate has no OCSP responder URL")
	// ErrUnknownStatus indicates the responder answered "unknown".
	ErrUnknownStatus = errors.New("revocation: OCSP responder returned unknown status")
	// ErrStaleResponse indicates a "good" OCSP response outside its validity
	// window: its nextUpdate has passed, its thisUpdate is in the future, or —
	// when it carries no nextUpdate — its thisUpdate is older than the maximum
	// response age. It is treated as indeterminate, so the [FailMode] decides the
	// outcome.
	ErrStaleResponse = errors.New("revocation: OCSP response is not within its validity window")
)

// responderStatusError carries a non-200 OCSP HTTP status so the retry filter
// can distinguish a retryable 5xx from a terminal 4xx.
type responderStatusError struct{ code int }

func (e *responderStatusError) Error() string {
	return fmt.Sprintf("revocation: OCSP responder returned status %d", e.code)
}

// shouldRetryOCSP retries only transient failures — network errors and HTTP 5xx.
// Parse errors and 4xx responses are terminal, so they fail fast.
func shouldRetryOCSP(err error) bool {
	if _, ok := errors.AsType[net.Error](err); ok {
		return true
	}
	if statusErr, ok := errors.AsType[*responderStatusError](err); ok {
		return statusErr.code >= http.StatusInternalServerError
	}
	return false
}

// Checker performs live OCSP revocation checks against a peer's leaf certificate
// and produces a [coremtls.CertValidator]. It is the network-backed counterpart
// to the in-memory [coremtls.RevocationList]: instead of a static denylist it
// queries the issuer's OCSP responder (the URL in the certificate's
// authorityInfoAccess), caches each result until the response's NextUpdate
// (for a response without one: until its ThisUpdate plus the maximum response
// age), and retries transient failures with backoff.
//
// An OCSP request is keyed by the issuer's name and public key, which the
// single-leaf validator seam does not carry — so the trusting issuer
// certificates are supplied at construction (in mTLS these are the same CA
// certificates configured as ClientCAs/RootCAs). Safe for concurrent use.
type Checker struct {
	issuers         []*x509.Certificate
	httpClient      *http.Client
	failMode        FailMode
	timeout         time.Duration
	maxAttempts     int
	maxTTL          time.Duration
	maxCacheEntries int
	clockSkew       time.Duration
	maxResponseAge  time.Duration
	now             func() time.Time

	mu    sync.RWMutex
	cache map[string]cacheEntry
	sf    singleflight.Group
}

type cacheEntry struct {
	revoked bool
	expiry  time.Time
}

// New builds a Checker that trusts the given issuer certificates. A certificate
// is matched to its issuer by Authority Key ID, falling back to the issuer DN.
func New(issuers []*x509.Certificate, opts ...Option) *Checker {
	o := newOptions(opts...)
	hc := o.httpClient
	if hc == nil {
		hc = &http.Client{}
	}
	now := o.now
	if now == nil {
		now = time.Now
	}
	return &Checker{
		issuers:         slices.Clone(issuers),
		httpClient:      hc,
		failMode:        o.failMode,
		timeout:         o.timeout,
		maxAttempts:     o.maxAttempts,
		maxTTL:          o.maxTTL,
		maxCacheEntries: o.maxCacheEntries,
		clockSkew:       o.clockSkew,
		maxResponseAge:  o.maxResponseAge,
		now:             now,
		cache:           make(map[string]cacheEntry),
	}
}

// Validator returns a [coremtls.CertValidator] that rejects a revoked peer with
// [coremtls.ErrRevoked]. Wire it with coremtls.WithValidator.
func (c *Checker) Validator() coremtls.CertValidator {
	return c.Check
}

// Check reports whether leaf is revoked. It returns [coremtls.ErrRevoked] when
// the responder confirms revocation, nil when the certificate is good, and — for
// an indeterminate result (no issuer, no responder, network failure, unknown or
// stale status) — either nil ([FailOpen]) or a wrapped error ([FailClosed]).
func (c *Checker) Check(leaf *x509.Certificate) error {
	if leaf == nil {
		return nil
	}
	issuer := c.issuerFor(leaf)
	if issuer == nil {
		return c.indeterminate(ErrNoIssuer)
	}
	if len(leaf.OCSPServer) == 0 {
		return c.indeterminate(ErrNoResponder)
	}

	key := cacheKey(issuer, leaf)
	if e, ok := c.lookup(key); ok {
		if e.revoked {
			return coremtls.ErrRevoked
		}
		return nil
	}

	revoked, err := c.checkOnce(key, leaf, issuer)
	if err != nil {
		return c.indeterminate(err)
	}
	if revoked {
		return coremtls.ErrRevoked
	}
	return nil
}

// checkOnce performs the OCSP query for key, deduplicating concurrent misses for
// the same certificate so a burst of handshakes triggers a single responder
// round-trip — the rest share its result instead of stampeding the responder.
func (c *Checker) checkOnce(key string, leaf, issuer *x509.Certificate) (bool, error) {
	v, err, _ := c.sf.Do(key, func() (any, error) {
		if e, ok := c.lookup(key); ok { // another caller may have just filled it
			return e.revoked, nil
		}
		revoked, expiry, qerr := c.queryOCSP(leaf, issuer)
		if qerr != nil {
			return false, qerr
		}
		c.store(key, revoked, expiry)
		return revoked, nil
	})
	if err != nil {
		return false, err
	}
	revoked, _ := v.(bool)
	return revoked, nil
}

// indeterminate applies the fail mode to a status that could not be confirmed.
func (c *Checker) indeterminate(err error) error {
	if c.failMode == FailClosed {
		return coreerrs.Wrap(err, "revocation: fail-closed")
	}
	return nil
}

// issuerFor returns the configured issuer that signed leaf, or nil.
func (c *Checker) issuerFor(leaf *x509.Certificate) *x509.Certificate {
	for _, iss := range c.issuers {
		if len(leaf.AuthorityKeyId) > 0 && len(iss.SubjectKeyId) > 0 {
			if bytes.Equal(leaf.AuthorityKeyId, iss.SubjectKeyId) {
				return iss
			}
			continue
		}
		if bytes.Equal(leaf.RawIssuer, iss.RawSubject) {
			return iss
		}
	}
	return nil
}

func (c *Checker) lookup(key string) (cacheEntry, bool) {
	c.mu.RLock()
	e, ok := c.cache[key]
	c.mu.RUnlock()
	if !ok || c.now().After(e.expiry) {
		return cacheEntry{}, false
	}
	return e, true
}

func (c *Checker) store(key string, revoked bool, expiry time.Time) {
	if !expiry.After(c.now()) {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.cache[key]; !exists && len(c.cache) >= c.maxCacheEntries {
		c.evictLocked()
	}
	c.cache[key] = cacheEntry{revoked: revoked, expiry: expiry}
}

// evictLocked drops expired entries to make room; if none are expired it drops
// one arbitrary entry, keeping the cache bounded by maxCacheEntries. The caller
// holds c.mu.
func (c *Checker) evictLocked() {
	now := c.now()
	for k, e := range c.cache {
		if now.After(e.expiry) {
			delete(c.cache, k)
		}
	}
	if len(c.cache) < c.maxCacheEntries {
		return
	}
	for k := range c.cache { // still full: evict one to make room
		delete(c.cache, k)
		return
	}
}

// queryOCSP fetches and parses the OCSP status, retrying transient failures.
// expiry is the absolute time until which the status may be cached.
func (c *Checker) queryOCSP(leaf, issuer *x509.Certificate) (revoked bool, expiry time.Time, err error) {
	reqDER, err := ocsp.CreateRequest(leaf, issuer, &ocsp.RequestOptions{Hash: crypto.SHA256})
	if err != nil {
		return false, time.Time{}, coreerrs.Wrap(err, "revocation: build OCSP request")
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	responder := leaf.OCSPServer[0]
	var resp *ocsp.Response
	err = retry.Do(ctx, func(ctx context.Context) error {
		body, e := c.post(ctx, responder, reqDER)
		if e != nil {
			return e
		}
		r, e := ocsp.ParseResponseForCert(body, leaf, issuer)
		if e != nil {
			return e
		}
		resp = r
		return nil
	},
		retry.WithMaxAttempts(c.maxAttempts),
		retry.WithShouldRetry(shouldRetryOCSP),
		retry.WithNextDelay(retry.Exponential(retry.ExponentialConfig{
			BaseDelay: retryBaseDelay,
			MaxDelay:  retryMaxDelay,
			Jitter:    retryJitter,
		})),
	)
	if err != nil {
		return false, time.Time{}, coreerrs.Wrap(err, "revocation: query OCSP")
	}

	switch resp.Status {
	case ocsp.Good:
		// The parser checks the signature, not freshness: a replayed "good"
		// response past its nextUpdate (dated in the future, or — without a
		// nextUpdate — older than maxResponseAge) must not vouch for the
		// certificate, nor be cached past that deadline.
		if err := c.checkFreshness(resp); err != nil {
			return false, time.Time{}, err
		}
		expiry := c.expiryFor(resp)
		if resp.NextUpdate.IsZero() {
			expiry = earliest(expiry, resp.ThisUpdate.Add(c.maxResponseAge))
		}
		return false, expiry, nil
	case ocsp.Revoked:
		// A signed revocation is honored even when stale: revocation is
		// permanent, and demoting it to indeterminate would accept the
		// certificate under FailOpen.
		return true, c.expiryFor(resp), nil
	default:
		return false, time.Time{}, ErrUnknownStatus
	}
}

func (c *Checker) post(ctx context.Context, url string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", ocspContentType)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, &responderStatusError{code: resp.StatusCode}
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxOCSPRespBytes))
}

// checkFreshness rejects a response whose validity window does not contain
// now, tolerating clockSkew on both ends. A response without a nextUpdate is
// valid until its thisUpdate plus maxResponseAge.
func (c *Checker) checkFreshness(resp *ocsp.Response) error {
	now := c.now()
	if resp.ThisUpdate.After(now.Add(c.clockSkew)) {
		return fmt.Errorf("%w: thisUpdate %s is in the future", ErrStaleResponse, resp.ThisUpdate.Format(time.RFC3339))
	}
	if !resp.NextUpdate.IsZero() && now.After(resp.NextUpdate.Add(c.clockSkew)) {
		return fmt.Errorf("%w: nextUpdate %s has passed", ErrStaleResponse, resp.NextUpdate.Format(time.RFC3339))
	}
	if resp.NextUpdate.IsZero() && now.After(resp.ThisUpdate.Add(c.maxResponseAge+c.clockSkew)) {
		return fmt.Errorf("%w: thisUpdate %s is older than the maximum response age %s without a nextUpdate",
			ErrStaleResponse, resp.ThisUpdate.Format(time.RFC3339), c.maxResponseAge)
	}
	return nil
}

// expiryFor derives the cache expiry from the response NextUpdate, capped at
// now+maxTTL. A zero NextUpdate yields the cap; an elapsed one yields a time in
// the past, which store treats as "do not cache".
func (c *Checker) expiryFor(resp *ocsp.Response) time.Time {
	limit := c.now().Add(c.maxTTL)
	if resp.NextUpdate.IsZero() {
		return limit
	}
	return earliest(resp.NextUpdate, limit)
}

// earliest returns the earlier of a and b.
func earliest(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// cacheKey scopes a serial number to its issuer so serials never collide
// across CAs.
func cacheKey(issuer, leaf *x509.Certificate) string {
	return leaf.SerialNumber.String() + "@" + string(issuer.RawSubject)
}
