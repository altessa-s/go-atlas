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
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas-core/encoding/keys"
	"github.com/altessa-s/go-atlas-core/retry"
	"github.com/altessa-s/go-atlas-core/runtime/singleflight"

	"golang.org/x/crypto/ocsp"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	httpclient "github.com/altessa-s/go-atlas-net/http/client"
	coremtls "github.com/altessa-s/go-atlas/auth/mtls"
)

const (
	ocspContentType  = "application/ocsp-request"
	maxOCSPRespBytes = 1 << 20 // 1 MiB ceiling on an OCSP response body.

	// OCSP retry backoff parameters.
	retryBaseDelay = 50 * time.Millisecond
	retryMaxDelay  = time.Second
	retryJitter    = 0.2

	// maxRedirects bounds the same-host redirects the default client follows.
	maxRedirects = 10

	// Default client transport parameters.
	dialKeepAlive       = 30 * time.Second
	idleConnTimeout     = 90 * time.Second
	maxIdleConns        = 16
	tlsHandshakeTimeout = 10 * time.Second
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
	// ErrCrossHostRedirect indicates the OCSP responder redirected the request to
	// a different host. The default HTTP client refuses to follow it, so the
	// responder named in the certificate cannot steer the check elsewhere.
	ErrCrossHostRedirect = errors.New("revocation: OCSP responder redirected to a different host")
	// ErrResponderNotAllowed indicates the responder URL in the certificate is
	// not an http or https URL, or names a host outside [WithAllowedResponders].
	// No request is sent.
	ErrResponderNotAllowed = errors.New("revocation: OCSP responder is not allowed")
	// ErrPrivateResponder indicates the default HTTP client refused to connect
	// to the responder because it resolves to a loopback, private, link-local,
	// unique-local, multicast, unspecified or otherwise non-public address. The
	// responder URL comes from the peer's certificate, so such a destination is
	// refused unless the host is listed with [WithAllowedResponders] or
	// [WithInsecureAllowPrivateResponders] opts out.
	ErrPrivateResponder = errors.New("revocation: OCSP responder resolves to a non-public address")
	// ErrUnauthorizedResponder indicates the OCSP response was signed by a
	// delegated certificate that is not an authorized responder (RFC 6960
	// section 4.2.2.2): it lacks the id-kp-OCSPSigning extended key usage or is
	// outside its validity period.
	ErrUnauthorizedResponder = errors.New("revocation: OCSP response signer is not an authorized responder")
	// errTooManyRedirects stops a same-host redirect loop.
	errTooManyRedirects = errors.New("revocation: OCSP responder redirected too many times")
)

// sameHostRedirect is the redirect policy of the default HTTP client: it follows
// a redirect only while it stays on the host (and port) of the original request.
func sameHostRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return errTooManyRedirects
	}
	if req.URL.Host != via[0].URL.Host {
		return ErrCrossHostRedirect
	}
	return nil
}

// responderStatusError carries a non-200 OCSP HTTP status so the retry filter
// can distinguish a retryable 5xx from a terminal 4xx.
type responderStatusError struct{ code int }

func (e *responderStatusError) Error() string {
	return fmt.Sprintf("revocation: OCSP responder returned status %d", e.code)
}

// shouldRetryOCSP reports a transient failure — a network error or HTTP 5xx —
// worth asking the same responder again. Parse errors, 4xx responses and
// refused destinations are final for that responder.
func shouldRetryOCSP(err error) bool {
	if errors.Is(err, ErrCrossHostRedirect) || errors.Is(err, errTooManyRedirects) || errors.Is(err, ErrPrivateResponder) {
		return false
	}
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
	issuers           []*x509.Certificate
	httpClient        *http.Client
	allowedResponders []string
	failMode          FailMode
	timeout           time.Duration
	attemptTimeout    time.Duration
	maxAttempts       int
	maxTTL            time.Duration
	maxCacheEntries   int
	clockSkew         time.Duration
	maxResponseAge    time.Duration
	now               func() time.Time

	mu    sync.RWMutex
	cache map[string]cacheEntry
	sf    *singleflight.Group[string, bool]
}

type cacheEntry struct {
	revoked bool
	expiry  time.Time
}

// New builds a Checker that trusts the given issuer certificates. A certificate
// is matched to its issuer by Authority Key ID, falling back to the issuer DN.
//
// Unless [WithFailMode] selects [FailOpen], a certificate whose status cannot be
// confirmed is rejected. Unless [WithHTTPClient] supplies one, the OCSP client
// is bounded by the check timeout, refuses a redirect to a different host and
// refuses to connect to a non-public address ([ErrPrivateResponder]) unless
// [WithAllowedResponders] or [WithInsecureAllowPrivateResponders] says
// otherwise.
func New(issuers []*x509.Certificate, opts ...Option) *Checker {
	o := newOptions(opts...)
	hc := o.httpClient
	if hc == nil {
		hc = newDefaultClient(o)
	}
	now := o.now
	if now == nil {
		now = time.Now
	}
	return &Checker{
		issuers:           slices.Clone(issuers),
		httpClient:        hc,
		allowedResponders: o.allowedResponders,
		failMode:          o.failMode,
		timeout:           o.timeout,
		attemptTimeout:    min(o.attemptTimeout, o.timeout),
		maxAttempts:       o.maxAttempts,
		maxTTL:            o.maxTTL,
		maxCacheEntries:   o.maxCacheEntries,
		clockSkew:         o.clockSkew,
		maxResponseAge:    o.maxResponseAge,
		now:               now,
		cache:             make(map[string]cacheEntry),
		// The group deadline is the check timeout; in-flight keys stay
		// unlimited, as every waiter is a handshake the server already admitted.
		sf: singleflight.New[string, bool](
			singleflight.WithTimeout(o.timeout),
			singleflight.WithMaxConcurrent(0),
		),
	}
}

// newDefaultClient builds the OCSP client used when none is supplied. Unless
// the responders are allowlisted or private responders are explicitly allowed,
// its dialer refuses a non-public address after DNS resolution and before the
// connect, so the refusal also covers redirects and DNS rebinding. Proxy
// environment variables are ignored while that guard is active: a proxied dial
// would be checked against the proxy address instead of the responder's.
func newDefaultClient(o *options) *http.Client {
	dialer := &net.Dialer{Timeout: min(o.attemptTimeout, o.timeout), KeepAlive: dialKeepAlive}
	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		ForceAttemptHTTP2:   true,
		MaxIdleConns:        maxIdleConns,
		IdleConnTimeout:     idleConnTimeout,
		TLSHandshakeTimeout: tlsHandshakeTimeout,
	}
	if len(o.allowedResponders) == 0 && !o.insecureAllowPrivateResponders {
		dialer.Control = httpclient.SSRFDialControl()
		transport.Proxy = nil
	}
	transport.DialContext = dialer.DialContext
	return &http.Client{Timeout: o.timeout, CheckRedirect: sameHostRedirect, Transport: transport}
}

// Validator returns a [coremtls.CertValidator] that rejects a revoked peer with
// [coremtls.ErrRevoked]. Wire it with coremtls.WithValidator, next to the
// authenticator's trust decision.
func (c *Checker) Validator() coremtls.CertValidator {
	return c.Check
}

// Check reports whether leaf is revoked. It returns [coremtls.ErrRevoked] when
// the responder confirms revocation, nil when the certificate is good, and — for
// an indeterminate result (no issuer, no responder, a refused responder,
// network failure, unknown or stale status, an unauthorized response signer,
// refused redirect) — either a wrapped error ([FailClosed], the default) or
// nil ([FailOpen]).
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
	responders, err := c.allowedResponderURLs(leaf.OCSPServer)
	if err != nil {
		return c.indeterminate(err)
	}

	revoked, err := c.checkOnce(key, leaf, issuer, responders)
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
func (c *Checker) checkOnce(key string, leaf, issuer *x509.Certificate, responders []string) (bool, error) {
	// The validator seam carries no context; the group deadline bounds the query.
	return c.sf.Do(context.Background(), key, func(ctx context.Context) (bool, error) {
		if e, ok := c.lookup(key); ok { // another caller may have just filled it
			return e.revoked, nil
		}
		revoked, expiry, qerr := c.queryOCSP(ctx, leaf, issuer, responders)
		if qerr != nil {
			return false, qerr
		}
		c.store(key, revoked, expiry)
		return revoked, nil
	})
}

// allowedResponderURLs returns the responder URLs of a certificate the checker
// may contact, in certificate order. When none passes, it returns the refusal
// of the first one.
func (c *Checker) allowedResponderURLs(responders []string) ([]string, error) {
	allowed := make([]string, 0, len(responders))
	var firstErr error
	for _, responder := range responders {
		if err := c.checkResponderURL(responder); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		allowed = append(allowed, responder)
	}
	if len(allowed) == 0 {
		return nil, firstErr
	}
	return allowed, nil
}

// checkResponderURL refuses a responder URL that is not http or https, or
// whose host is outside the responder allowlist when one is configured.
func (c *Checker) checkResponderURL(responder string) error {
	u, err := url.Parse(responder)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return fmt.Errorf("%w: %q is not an http or https URL", ErrResponderNotAllowed, responder)
	}
	if len(c.allowedResponders) == 0 {
		return nil
	}
	if slices.Contains(c.allowedResponders, strings.ToLower(u.Hostname())) ||
		slices.Contains(c.allowedResponders, strings.ToLower(u.Host)) {
		return nil
	}
	return fmt.Errorf("%w: host %q is not listed in WithAllowedResponders", ErrResponderNotAllowed, u.Host)
}

// indeterminate applies the fail mode to a status that could not be confirmed.
func (c *Checker) indeterminate(err error) error {
	if c.failMode == FailClosed {
		return coreerrs.Wrap(err, "revocation: fail-closed")
	}
	return nil
}

// issuerFor returns the configured issuer that signed leaf, or nil. A key
// identifier match anywhere in the list wins over a subject match, so an issuer
// without a SubjectKeyId cannot shadow the one the leaf names — as happens in a
// CA key rollover, where the old and the new CA share a subject DN. Without a
// deciding key identifier the leaf is matched by issuer DN; when several
// issuers share that DN, the one whose key verifies the leaf's signature is
// chosen.
func (c *Checker) issuerFor(leaf *x509.Certificate) *x509.Certificate {
	if len(leaf.AuthorityKeyId) > 0 {
		for _, iss := range c.issuers {
			if bytes.Equal(leaf.AuthorityKeyId, iss.SubjectKeyId) {
				return iss
			}
		}
	}
	var candidates []*x509.Certificate
	for _, iss := range c.issuers {
		// Both identifiers present means they were compared above and differ.
		if len(leaf.AuthorityKeyId) > 0 && len(iss.SubjectKeyId) > 0 {
			continue
		}
		if bytes.Equal(leaf.RawIssuer, iss.RawSubject) {
			candidates = append(candidates, iss)
		}
	}
	switch len(candidates) {
	case 0:
		return nil
	case 1:
		return candidates[0]
	}
	for _, iss := range candidates {
		if iss.CheckSignature(leaf.SignatureAlgorithm, leaf.RawTBSCertificate, leaf.Signature) == nil {
			return iss
		}
	}
	return candidates[0]
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
// Every request is bounded by the attempt timeout and the whole query by the
// check timeout, so one stalled request does not use up the retries. Attempts
// rotate through responders, and a certificate naming more responders than
// the retry count allows still has each of them tried once. A responder that
// fails to give a definitive answer for any other reason is dropped and the
// next one is asked; when none answers, the per-responder errors are joined.
// A revoked status from any responder is final. expiry is the
// absolute time until which the status may be cached.
func (c *Checker) queryOCSP(
	ctx context.Context, leaf, issuer *x509.Certificate, responders []string,
) (revoked bool, expiry time.Time, err error) {
	reqDER, err := ocsp.CreateRequest(leaf, issuer, &ocsp.RequestOptions{Hash: crypto.SHA256})
	if err != nil {
		return false, time.Time{}, coreerrs.Wrap(err, "revocation: build OCSP request")
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// live holds the responders still worth asking, next the one to ask. A
	// transient failure keeps a responder in the rotation; any other failure to
	// get a definitive answer from it drops it, and the query goes on while a
	// responder remains.
	var (
		live = slices.Clone(responders)
		next int
		errs []error
	)
	err = retry.Do(ctx, func(ctx context.Context) error {
		next %= len(live)
		responder := live[next]
		attemptCtx, cancelAttempt := context.WithTimeout(ctx, c.attemptTimeout)
		defer cancelAttempt()
		r, exp, e := c.queryResponder(attemptCtx, responder, reqDER, leaf, issuer)
		if e == nil {
			revoked, expiry = r, exp
			return nil
		}
		errs = append(errs, fmt.Errorf("responder %s: %w", responder, e))
		if shouldRetryOCSP(e) {
			next++
		} else {
			live = slices.Delete(live, next, next+1)
		}
		return e
	},
		retry.WithMaxAttempts(max(c.maxAttempts, len(responders)-1)),
		retry.WithShouldRetry(func(error) bool { return len(live) > 0 }),
		retry.WithNextDelay(retry.Exponential(
			retryBaseDelay,
			retry.WithExponentialMaxDelay(retryMaxDelay),
			retry.WithExponentialJitter(retryJitter),
		)),
	)
	if err != nil {
		if joined := errors.Join(append(errs, ctx.Err())...); joined != nil {
			err = joined
		}
		return false, time.Time{}, coreerrs.Wrap(err, "revocation: query OCSP")
	}
	return revoked, expiry, nil
}

// queryResponder asks one responder and returns its answer only when it is
// definitive: a response for leaf signed by an authorized signer, saying
// revoked, or saying good within its validity window. Anything else — a
// transport failure, an unparseable or unauthorized response, a stale or
// unknown status — is an error, so the caller can ask another responder.
func (c *Checker) queryResponder(
	ctx context.Context, responder string, reqDER []byte, leaf, issuer *x509.Certificate,
) (revoked bool, expiry time.Time, err error) {
	body, err := c.post(ctx, responder, reqDER)
	if err != nil {
		return false, time.Time{}, err
	}
	resp, err := ocsp.ParseResponseForCert(body, leaf, issuer)
	if err != nil {
		return false, time.Time{}, err
	}
	if err := c.checkResponseSigner(resp, issuer); err != nil {
		return false, time.Time{}, err
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
		if errors.Is(err, httpclient.ErrSSRFBlocked) {
			return nil, fmt.Errorf("%w: list the responder host with WithAllowedResponders, "+
				"or opt out with WithInsecureAllowPrivateResponders: %w", ErrPrivateResponder, err)
		}
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, &responderStatusError{code: resp.StatusCode}
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxOCSPRespBytes))
}

// checkResponseSigner enforces RFC 6960 section 4.2.2.2 on a response signed
// by a delegated certificate. The parser verifies that the embedded
// certificate was issued by issuer and that it signed the response, but not
// that the issuer authorized it to sign OCSP responses: without this check any
// certificate from the same issuer could vouch for a revoked one. A response
// signed with the issuer's own key needs no delegation.
func (c *Checker) checkResponseSigner(resp *ocsp.Response, issuer *x509.Certificate) error {
	signer := resp.Certificate
	if signer == nil || bytes.Equal(signer.RawSubjectPublicKeyInfo, issuer.RawSubjectPublicKeyInfo) {
		return nil
	}
	if !slices.Contains(signer.ExtKeyUsage, x509.ExtKeyUsageOCSPSigning) {
		return fmt.Errorf("%w: certificate %q lacks the OCSP signing extended key usage",
			ErrUnauthorizedResponder, signer.Subject.String())
	}
	now := c.now()
	if now.Add(c.clockSkew).Before(signer.NotBefore) || now.Add(-c.clockSkew).After(signer.NotAfter) {
		return fmt.Errorf("%w: certificate %q is outside its validity period",
			ErrUnauthorizedResponder, signer.Subject.String())
	}
	return nil
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

// cacheKey scopes a serial number to the issuer's name and public key — the
// material an OCSP CertID identifies a certificate by — so serials never
// collide across CAs, including CAs that share a subject DN.
func cacheKey(issuer, leaf *x509.Certificate) string {
	return string(keys.Digest(issuer.RawSubject, issuer.RawSubjectPublicKeyInfo, []byte(leaf.SerialNumber.String())))
}
