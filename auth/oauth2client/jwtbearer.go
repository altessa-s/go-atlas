// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/altessa-s/go-atlas-core/time/clock"
	"github.com/altessa-s/go-atlas/auth/jwt"

	"golang.org/x/oauth2"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// GrantTypeJWTBearer is the RFC 7523 §2.1 grant_type value of the JWT bearer
// authorization grant.
const GrantTypeJWTBearer = "urn:ietf:params:oauth:grant-type:jwt-bearer" // #nosec G101 -- RFC 7523 grant_type URI, not a credential

// grantJWTBearer is the metrics label of the JWT bearer grant.
const grantJWTBearer = "jwt_bearer"

// jwtBearerAssertionLifetime is the validity of a minted assertion. One hour is
// the maximum Google and most IdPs accept; the assertion is minted fresh for
// every token request, so the window only bounds replay of a leaked assertion.
const jwtBearerAssertionLifetime = time.Hour

// defaultJWTBearerEarlyExpiry is the refresh margin when [WithEarlyExpiry] is
// unset, matching x/oauth2's default expiry window.
const defaultJWTBearerEarlyExpiry = 10 * time.Second

// ErrJWTBearerConfig is returned by [NewJWTBearer] for an unusable
// configuration: an invalid token URL, an empty issuer, a missing key, or a key
// whose algorithm is not asymmetric or does not match its key material.
var ErrJWTBearerConfig = errors.New("auth/oauth2client: invalid jwt-bearer grant configuration")

// JWTBearer acquires access tokens with the JWT bearer authorization grant
// (RFC 7523 §2.1): it signs an assertion (iss = issuer, aud = token URL, scope,
// iat, exp) with the caller's private key and trades it for an access token at
// the token endpoint. It is the flow of Google service accounts and similar
// "signed assertion, no client secret" IdPs.
//
// Unlike x/oauth2/jwt, every exchange runs on the context of the [JWTBearer.Token]
// call, so a caller's deadline and cancellation bound the token request. The
// issued token is cached until [WithEarlyExpiry] before its expiry; a response
// without a positive expires_in is rejected, because such a token would never be
// refreshed. Refreshes are serialized: concurrent callers that find the cache
// stale wait for one exchange instead of each starting their own.
//
// A JWTBearer is safe for concurrent use. Build one per credential and keep it
// for the credential's lifetime.
type JWTBearer struct {
	tokenURL string
	issuer   string
	scope    string
	key      jwt.SigningKey
	signer   *jwt.Signer
	client   *http.Client
	retry    retryPolicy
	early    time.Duration
	clock    clock.Clock
	metrics  *Metrics
	logger   *slog.Logger

	// refresh is a one-slot semaphore serializing exchanges. A channel rather
	// than a mutex lets a waiter give up when its context ends.
	refresh chan struct{}
	// token is the last issued token; nil until the first exchange succeeds.
	token atomic.Pointer[cachedToken]
}

// cachedToken is an issued token with the instant it stops being served.
type cachedToken struct {
	tok       *oauth2.Token
	refreshAt time.Time
}

// newCachedToken caches tok issued at now. The early-expiry window is capped at
// half the token's lifetime, so a token that lives shorter than the window is
// still served for a while instead of forcing an exchange on every call.
func newCachedToken(tok *oauth2.Token, now time.Time, early time.Duration) *cachedToken {
	const maxEarlyShare = 2 // the window covers at most 1/maxEarlyShare of the lifetime
	return &cachedToken{tok: tok, refreshAt: tok.Expiry.Add(-min(early, tok.Expiry.Sub(now)/maxEarlyShare))}
}

// NewJWTBearer builds a [JWTBearer] for the token endpoint at tokenURL.
// issuer is the assertion's iss claim (for a Google service account, its
// client_email); key signs the assertion and must use an asymmetric algorithm
// (for Google, [jwt.AlgRS256] with the account's RSA key). An empty key.KeyID
// is allowed and omits the kid header.
//
// [WithScopes] sets the assertion's scope claim (space-delimited);
// [WithHTTPClient], [WithEarlyExpiry], [WithRetryAttempts], [WithMetrics],
// [WithLogger], and [WithClock] apply as well. The configuration is validated by
// signing a probe assertion, so an unusable key fails here with
// [ErrJWTBearerConfig] rather than on the first token request. A tokenURL that
// is not https (plain http is accepted on a loopback host, or anywhere with
// [WithInsecureHTTP]) fails here too, with an error that also wraps
// [ErrInsecureEndpoint].
func NewJWTBearer(tokenURL, issuer string, key jwt.SigningKey, opts ...Option) (*JWTBearer, error) {
	o := newOptions(opts...)
	if err := o.validateEndpoint("token URL", tokenURL); err != nil {
		return nil, errors.Join(ErrJWTBearerConfig, err)
	}
	if issuer == "" {
		return nil, coreerrs.Wrap(ErrJWTBearerConfig, "issuer is required")
	}
	if key.Key == nil {
		return nil, coreerrs.Wrap(ErrJWTBearerConfig, "signing key is required")
	}
	if !slices.Contains(jwt.DefaultAllowedAlgorithms(), key.Algorithm) {
		return nil, coreerrs.Wrapf(ErrJWTBearerConfig, "algorithm %q is not an allowed asymmetric algorithm", key.Algorithm)
	}

	signerOpts := []jwt.Option{jwt.WithAllowedAlgorithms(key.Algorithm), jwt.WithClock(o.clock)}
	if key.KeyID == "" {
		signerOpts = append(signerOpts, jwt.WithAllowEmptyKeyID())
	}
	b := &JWTBearer{
		tokenURL: tokenURL,
		issuer:   issuer,
		scope:    strings.Join(o.scopes, " "),
		key:      key,
		signer:   jwt.NewSigner(signerOpts...),
		client:   o.client(),
		retry:    o.retryPolicy(),
		early:    o.earlyExpiry,
		clock:    o.clock,
		metrics:  o.metrics,
		logger:   o.logger,
		refresh:  make(chan struct{}, 1),
	}
	if b.early <= 0 {
		b.early = defaultJWTBearerEarlyExpiry
	}
	if _, err := b.assertion(o.clock.Now()); err != nil {
		return nil, coreerrs.Wrap(errors.Join(ErrJWTBearerConfig, err), "sign probe assertion")
	}
	return b, nil
}

// Token returns a valid access token, exchanging a fresh assertion when the
// cached token is missing or within the early-expiry window of its expiry. The
// returned token is shared with other callers and must not be modified.
//
// The exchange runs on ctx. A caller that finds another exchange in flight
// waits for it and then reuses its token; if that exchange fails, the waiter
// runs its own on its own ctx. A ctx that ends while waiting or exchanging
// returns an error matching ctx's error ([context.Canceled] or
// [context.DeadlineExceeded]). A cached token is returned even when ctx is
// already done, since serving it needs no I/O.
//
// Errors wrap [ErrTokenRequest] for a signing or transport failure and for a
// non-2xx response — the latter also carries an [*oauth2.RetrieveError] with
// the status and the IdP's error code, reachable via errors.As. A body that is
// unparsable, lacks access_token, or lacks a positive expires_in wraps
// [ErrTokenExchange]. Failures are never cached.
func (b *JWTBearer) Token(ctx context.Context) (*oauth2.Token, error) {
	if tok := b.fresh(); tok != nil {
		return tok, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, coreerrs.Wrap(err, "jwt-bearer token")
	}
	select {
	case b.refresh <- struct{}{}:
	case <-ctx.Done():
		return nil, coreerrs.Wrap(ctx.Err(), "wait for jwt-bearer token refresh")
	}
	defer func() { <-b.refresh }()

	// Another caller may have refreshed while this one waited.
	if tok := b.fresh(); tok != nil {
		return tok, nil
	}

	start := time.Now()
	tok, err := withRetry(ctx, b.retry, grantJWTBearer, "jwt-bearer token request", b.metrics, b.logger, b.fetch)
	b.metrics.recordFetch(grantJWTBearer, err == nil, time.Since(start))
	if err != nil {
		if b.logger != nil {
			b.logger.Warn("oauth2client: jwt-bearer token request failed", "err", logError(err))
		}
		return nil, err
	}
	b.token.Store(newCachedToken(tok, b.clock.Now(), b.early))
	return tok, nil
}

// TokenSource adapts b to [oauth2.TokenSource], running every refresh on ctx
// the way the other helpers pin their context. Prefer [JWTBearer.Token] with a
// per-call context where the caller has one.
func (b *JWTBearer) TokenSource(ctx context.Context) oauth2.TokenSource {
	return &jwtBearerSource{bearer: b, ctx: ctx}
}

// fresh returns the cached token when it is outside its early-expiry window,
// or nil.
func (b *JWTBearer) fresh() *oauth2.Token {
	c := b.token.Load()
	if c == nil || !b.clock.Now().Before(c.refreshAt) {
		return nil
	}
	return c.tok
}

// fetch performs one token request with a freshly minted assertion. Transport
// errors, 429 and 5xx are retryable [*exchangeError] values.
func (b *JWTBearer) fetch(ctx context.Context) (*oauth2.Token, error) {
	now := b.clock.Now()
	assertion, err := b.assertion(now)
	if err != nil {
		return nil, &exchangeError{cause: coreerrs.Wrap(errors.Join(ErrTokenRequest, err), "sign jwt-bearer assertion")}
	}
	form := url.Values{}
	form.Set("grant_type", GrantTypeJWTBearer)
	form.Set("assertion", assertion)

	status, body, err := doForm(ctx, b.client, b.tokenURL, form, "", "")
	if err != nil {
		return nil, &exchangeError{
			retryable: !permanentRequestError(err),
			cause:     coreerrs.Wrap(errors.Join(ErrTokenRequest, err), "jwt-bearer token request"),
		}
	}
	if status < 200 || status >= 300 {
		return nil, &exchangeError{
			retryable: status == http.StatusTooManyRequests || status >= 500,
			cause:     coreerrs.JoinWrap(ErrTokenRequest, responseError(retrieveError(status, body), status, len(body))),
		}
	}
	tok, err := parseTokenResponse(status, body, now)
	if err != nil {
		return nil, err
	}
	if tok.Expiry.IsZero() {
		return nil, coreerrs.Wrap(ErrTokenExchange, "response missing a positive expires_in")
	}
	return tok, nil
}

// assertion signs the grant assertion valid from now.
func (b *JWTBearer) assertion(now time.Time) (string, error) {
	claims := jwt.Claims{
		"iss": b.issuer,
		"aud": b.tokenURL,
		"iat": now.Unix(),
		"exp": now.Add(jwtBearerAssertionLifetime).Unix(),
	}
	if b.scope != "" {
		claims["scope"] = b.scope
	}
	return b.signer.Sign(b.key, claims)
}

// retrieveError describes a non-2xx token response as an
// [*oauth2.RetrieveError], the error currency of the x/oauth2-backed grants.
// The body and RFC 6749 error fields remain available through errors.As.
func retrieveError(status int, body []byte) *oauth2.RetrieveError {
	re := &oauth2.RetrieveError{
		Response: &http.Response{StatusCode: status, Status: fmt.Sprintf("%d %s", status, http.StatusText(status))},
		Body:     body,
	}
	var e struct {
		Code        string `json:"error"`
		Description string `json:"error_description"`
		URI         string `json:"error_uri"`
	}
	if json.Unmarshal(body, &e) == nil {
		re.ErrorCode = e.Code
		re.ErrorDescription = e.Description
		re.ErrorURI = e.URI
	}
	return re
}

// jwtBearerSource adapts a [JWTBearer] to [oauth2.TokenSource] with a pinned
// context.
type jwtBearerSource struct {
	bearer *JWTBearer
	ctx    context.Context //nolint:containedctx // mirrors x/oauth2 TokenSource pinning ctx for background refresh
}

// Token returns the bearer's token for the pinned context.
func (s *jwtBearerSource) Token() (*oauth2.Token, error) { return s.bearer.Token(s.ctx) }
