// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// ClientCredentials returns a self-refreshing [oauth2.TokenSource] for the
// client_credentials grant (RFC 6749 §4.4) — the standard machine-to-machine
// flow where a service authenticates as itself, with no end user involved.
//
// tokenURL is the IdP token endpoint; clientID and clientSecret are the
// service's own credentials. The returned source caches the access token and
// silently fetches a new one from ctx once it expires, so it is safe to reuse
// for the process lifetime. It plugs directly into
// [github.com/altessa-s/go-atlas/transport/grpc/client.NewInsecureTokenCredentials]
// and any other consumer of [oauth2.TokenSource].
//
// Configure scopes, a custom [http.Client], the credential [oauth2.AuthStyle],
// or extra endpoint params with the [Option] values. When a [ClientAuthenticator]
// is set via [WithClientAuth], the client authenticates with a signed JWT
// assertion (private_key_jwt / client_secret_jwt) and clientSecret is unused.
func ClientCredentials(ctx context.Context, tokenURL, clientID, clientSecret string, opts ...Option) oauth2.TokenSource {
	o := newOptions(opts...)
	if o.clientAuth != nil {
		src := &ccAssertionSource{
			ctx:            ctx,
			tokenURL:       tokenURL,
			scopes:         o.scopes,
			endpointParams: o.endpointParams,
			clientAuth:     o.clientAuth,
			client:         o.httpClient,
		}
		return measure(reuse(nil, src, o.earlyExpiry), grantClientCredentials, o.metrics, o.logger)
	}
	cfg := &clientcredentials.Config{
		ClientID:       clientID,
		ClientSecret:   clientSecret,
		TokenURL:       tokenURL,
		Scopes:         o.scopes,
		EndpointParams: o.endpointParams,
		AuthStyle:      o.authStyle,
	}
	ctx = withHTTPClient(ctx, o.httpClient)
	if o.earlyExpiry > 0 {
		// clientcredentials.Config.TokenSource bakes in x/oauth2's default expiry
		// window; wrap our own one-shot to honor the configured earlyExpiry.
		inner := &ccStdSource{ctx: ctx, cfg: cfg}
		return measure(oauth2.ReuseTokenSourceWithExpiry(nil, inner, o.earlyExpiry),
			grantClientCredentials, o.metrics, o.logger)
	}
	return measure(cfg.TokenSource(ctx), grantClientCredentials, o.metrics, o.logger)
}

// reuse wraps src in a caching token source, honoring earlyExpiry (refresh this
// long before exp) when positive and otherwise using x/oauth2's default window.
func reuse(tok *oauth2.Token, src oauth2.TokenSource, earlyExpiry time.Duration) oauth2.TokenSource {
	if earlyExpiry > 0 {
		return oauth2.ReuseTokenSourceWithExpiry(tok, src, earlyExpiry)
	}
	return oauth2.ReuseTokenSource(tok, src)
}

// ccStdSource is a one-shot client_credentials fetcher over
// [clientcredentials.Config], used only to apply a custom early-expiry window.
type ccStdSource struct {
	ctx context.Context //nolint:containedctx // pinned for background refresh, mirrors x/oauth2 TokenSource
	cfg *clientcredentials.Config
}

// Token fetches one token from the configured endpoint.
func (s *ccStdSource) Token() (*oauth2.Token, error) { return s.cfg.Token(s.ctx) }

// ccAssertionSource fetches client_credentials tokens authenticating with a JWT
// assertion (RFC 7523). x/oauth2's clientcredentials.Config carries static
// EndpointParams and cannot mint a fresh assertion per fetch, so this hand-rolls
// the request; wrap it in [oauth2.ReuseTokenSource] for caching.
type ccAssertionSource struct {
	ctx            context.Context //nolint:containedctx // pinned for background refresh, mirrors x/oauth2 TokenSource
	tokenURL       string
	scopes         []string
	endpointParams url.Values
	clientAuth     ClientAuthenticator
	client         *http.Client
}

// Token performs one client_credentials fetch with a fresh client assertion.
func (s *ccAssertionSource) Token() (*oauth2.Token, error) {
	form := url.Values{}
	form.Set("grant_type", grantClientCredentials)
	if len(s.scopes) > 0 {
		form.Set("scope", strings.Join(s.scopes, " "))
	}
	for key, vals := range s.endpointParams {
		for _, val := range vals {
			form.Add(key, val)
		}
	}
	// The authenticator is always a client assertion here, so it merges its
	// params into the body and returns no Basic credentials.
	if _, _, err := applyClientAuth(s.ctx, form, s.clientAuth, 0, "", "", s.tokenURL); err != nil {
		return nil, err
	}

	status, body, err := doForm(s.ctx, s.client, s.tokenURL, form, "", "")
	if err != nil {
		return nil, coreerrs.Wrap(errors.Join(ErrTokenRequest, err), "client_credentials token request")
	}
	if status < 200 || status >= 300 {
		return nil, coreerrs.Wrapf(ErrTokenRequest, "unexpected status %d: %s", status, sanitizeErrorBody(body))
	}
	return parseTokenResponse(body)
}

// Refresh returns a self-refreshing [oauth2.TokenSource] seeded from an existing
// refresh token (RFC 6749 §6). The first call to Token exchanges refreshToken
// for a fresh access token at tokenURL; subsequent calls reuse it until expiry
// and then refresh again, rotating to a new refresh token when the IdP issues
// one.
//
// Use this to resume a previously authorized session (for example an
// authorization-code grant whose refresh token was persisted) without
// re-prompting the user.
func Refresh(ctx context.Context, tokenURL, clientID, clientSecret, refreshToken string, opts ...Option) oauth2.TokenSource {
	o := newOptions(opts...)
	cfg := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     oauth2.Endpoint{TokenURL: tokenURL, AuthStyle: o.authStyle},
		Scopes:       o.scopes,
	}
	src := cfg.TokenSource(withHTTPClient(ctx, o.httpClient), &oauth2.Token{RefreshToken: refreshToken})
	return measure(src, grantRefresh, o.metrics, o.logger)
}

// AuthCode drives the authorization_code grant (RFC 6749 §4.1), the flow used
// when a human authorizes the service at the IdP's consent screen. Build the
// consent URL with [AuthCode.AuthCodeURL], redirect the user to it, then trade
// the code the IdP redirects back for a self-refreshing token source with
// [AuthCode.Exchange].
//
// An AuthCode is immutable after construction and safe for concurrent use.
type AuthCode struct {
	cfg     oauth2.Config
	client  *http.Client
	metrics *Metrics
	logger  *slog.Logger
}

// NewAuthCode builds an [AuthCode] for the given IdP endpoint and registered
// client. endpoint carries the IdP's authorization and token URLs (and, if the
// IdP is picky, its own [oauth2.AuthStyle]); redirectURL is the callback the
// IdP redirects back to and must match the client registration. Set the
// requested scopes and a custom [http.Client] with the [Option] values.
func NewAuthCode(endpoint oauth2.Endpoint, clientID, clientSecret, redirectURL string, opts ...Option) *AuthCode {
	o := newOptions(opts...)
	return &AuthCode{
		cfg: oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint:     endpoint,
			RedirectURL:  redirectURL,
			Scopes:       o.scopes,
		},
		client:  o.httpClient,
		metrics: o.metrics,
		logger:  o.logger,
	}
}

// AuthCodeURL returns the IdP consent URL the user must visit. state is an
// opaque, unguessable value echoed back on redirect; verify it there to defend
// against CSRF. Pass extra parameters (PKCE challenge, "prompt", "audience")
// as [oauth2.AuthCodeOption] values such as [oauth2.S256ChallengeOption].
func (a *AuthCode) AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string {
	return a.cfg.AuthCodeURL(state, opts...)
}

// Exchange trades the authorization code the IdP redirected back for tokens.
// It returns a self-refreshing [oauth2.TokenSource] (backed by the granted
// refresh token) for ongoing use, plus the raw [oauth2.Token] for callers that
// need the initial access token, id_token, or refresh token immediately. Pass
// the PKCE verifier as [oauth2.VerifierOption] when the consent URL carried a
// challenge.
func (a *AuthCode) Exchange(
	ctx context.Context, code string, opts ...oauth2.AuthCodeOption,
) (oauth2.TokenSource, *oauth2.Token, error) {
	ctx = withHTTPClient(ctx, a.client)
	start := time.Now()
	tok, err := a.cfg.Exchange(ctx, code, opts...)
	a.metrics.recordFetch(grantAuthorizationCode, err == nil, time.Since(start))
	if err != nil {
		if a.logger != nil {
			a.logger.Warn("oauth2client: authorization-code exchange failed", "err", err)
		}
		return nil, nil, err
	}
	return measure(a.cfg.TokenSource(ctx, tok), grantAuthorizationCode, a.metrics, a.logger), tok, nil
}
