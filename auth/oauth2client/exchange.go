// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/altessa-s/go-atlas/core/retry"

	"golang.org/x/oauth2"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	coreio "github.com/altessa-s/go-atlas/core/io"
)

// GrantTypeTokenExchange is the RFC 8693 grant_type value.
const GrantTypeTokenExchange = "urn:ietf:params:oauth:grant-type:token-exchange"

// Token type identifiers for the subject_token_type, actor_token_type, and
// requested_token_type parameters (RFC 8693 §3).
const (
	TokenTypeAccessToken  = "urn:ietf:params:oauth:token-type:access_token"
	TokenTypeRefreshToken = "urn:ietf:params:oauth:token-type:refresh_token"
	TokenTypeIDToken      = "urn:ietf:params:oauth:token-type:id_token"
	TokenTypeSAML2        = "urn:ietf:params:oauth:token-type:saml2"
	TokenTypeJWT          = "urn:ietf:params:oauth:token-type:jwt"
)

// maxExchangeResponseSize caps the bytes read from the token endpoint. A token
// response is well under 64 KiB; anything larger signals a misbehaving upstream.
const maxExchangeResponseSize = 1 << 16 // 64 KiB

// maxErrorBodyLen bounds how much of an upstream error body is echoed into an
// error message, to avoid leaking oversized or sensitive payloads into logs.
const maxErrorBodyLen = 256

// ExchangeRequest describes a single RFC 8693 token exchange: trade a subject
// token (and optionally an actor token, for delegation) for a new token scoped
// to a downstream audience or resource. Only SubjectToken is required; empty
// optional fields are omitted from the request.
type ExchangeRequest struct {
	// SubjectToken is the token representing the identity on whose behalf the
	// request is made — typically the inbound token of the current call.
	SubjectToken string
	// SubjectTokenType identifies SubjectToken. Empty defaults to
	// [TokenTypeAccessToken].
	SubjectTokenType string
	// ActorToken represents the acting party in a delegation exchange. Empty
	// omits actor_token (and actor_token_type) entirely.
	ActorToken string
	// ActorTokenType identifies ActorToken. Empty, when ActorToken is set,
	// defaults to [TokenTypeAccessToken].
	ActorTokenType string
	// Resource is the target service's URI the issued token is scoped to.
	Resource string
	// Audience is the logical name of the target service (alternative to
	// Resource).
	Audience string
	// Scopes are the requested scopes for the issued token.
	Scopes []string
	// RequestedTokenType is the desired type of the issued token (for example
	// [TokenTypeAccessToken]). Empty lets the IdP choose.
	RequestedTokenType string
}

// Exchanger performs RFC 8693 token exchanges against a single IdP token
// endpoint. x/oauth2 has no built-in token-exchange grant, so this is a thin
// spec-compliant client that yields the same [oauth2.Token] currency as the
// other helpers. An Exchanger is immutable and safe for concurrent use.
type Exchanger struct {
	tokenURL       string
	clientID       string
	clientSecret   string
	authStyle      oauth2.AuthStyle
	endpointParams url.Values
	client         *http.Client
	retryAttempts  int
	retryBaseDelay time.Duration
	retryMaxDelay  time.Duration
	metrics        *Metrics
	logger         *slog.Logger
	clientAuth     ClientAuthenticator
	earlyExpiry    time.Duration
}

// NewExchanger builds an [Exchanger] for the IdP token endpoint at tokenURL.
// clientID and clientSecret authenticate this client to the endpoint; pass ""
// for both when the endpoint accepts unauthenticated (public) clients. Set the
// credential [oauth2.AuthStyle], extra endpoint params, or a custom
// [http.Client] with the [Option] values (scopes are taken per-request from
// [ExchangeRequest]).
func NewExchanger(tokenURL, clientID, clientSecret string, opts ...Option) *Exchanger {
	o := newOptions(opts...)
	return &Exchanger{
		tokenURL:       tokenURL,
		clientID:       clientID,
		clientSecret:   clientSecret,
		authStyle:      o.authStyle,
		endpointParams: o.endpointParams,
		client:         o.httpClient,
		retryAttempts:  o.retryAttempts,
		retryBaseDelay: o.retryBaseDelay,
		retryMaxDelay:  o.retryMaxDelay,
		metrics:        o.metrics,
		logger:         o.logger,
		clientAuth:     o.clientAuth,
		earlyExpiry:    o.earlyExpiry,
	}
}

// Exchange performs a token exchange and returns the issued token. The
// issued_token_type and granted scope, when present, are attached to the token
// and readable via [oauth2.Token.Extra] under "issued_token_type" and "scope".
// A missing subject token returns [ErrSubjectTokenRequired]; a transport
// failure or non-2xx response returns an error wrapping [ErrTokenExchange].
//
// When configured with [WithRetryAttempts], transient failures (transport
// errors, HTTP 429, and 5xx) are retried with exponential backoff; a 4xx other
// than 429 and a malformed response are returned immediately.
func (e *Exchanger) Exchange(ctx context.Context, req ExchangeRequest) (*oauth2.Token, error) {
	if req.SubjectToken == "" {
		return nil, ErrSubjectTokenRequired
	}

	start := time.Now()
	tok, err := e.exchange(ctx, req)
	e.metrics.recordFetch(grantTokenExchange, err == nil, time.Since(start))
	if err != nil && e.logger != nil {
		e.logger.Warn("oauth2client: token exchange failed", "err", err)
	}
	return tok, err
}

// exchange runs a single exchange or, when configured, a retrying one.
func (e *Exchanger) exchange(ctx context.Context, req ExchangeRequest) (*oauth2.Token, error) {
	if e.retryAttempts <= 0 {
		return e.exchangeOnce(ctx, req)
	}

	var tok *oauth2.Token
	err := retry.Do(ctx, func(ctx context.Context) error {
		var err error
		tok, err = e.exchangeOnce(ctx, req)
		return err
	},
		retry.WithMaxAttempts(e.retryAttempts),
		retry.WithNextDelay(retry.Exponential(retry.ExponentialConfig{
			BaseDelay: e.retryBaseDelay,
			MaxDelay:  e.retryMaxDelay,
		})),
		retry.WithShouldRetry(func(err error) bool {
			var ee *exchangeError
			return errors.As(err, &ee) && ee.retryable
		}),
		retry.WithOnRetry(func(attempt int, err error, delay time.Duration) {
			e.metrics.recordRetry(grantTokenExchange)
			if e.logger != nil {
				e.logger.Warn("oauth2client: retrying token exchange",
					"attempt", attempt+1, "delay", delay, "err", err)
			}
		}),
	)
	return tok, err
}

// exchangeOnce performs a single token-exchange round trip. Retryable failures
// are returned as an [*exchangeError] with retryable set.
func (e *Exchanger) exchangeOnce(ctx context.Context, req ExchangeRequest) (*oauth2.Token, error) {
	form := e.formValues(req)
	basicUser, basicPass, err := applyClientAuth(ctx, form, e.clientAuth, e.authStyle, e.clientID, e.clientSecret, e.tokenURL)
	if err != nil {
		return nil, &exchangeError{cause: coreerrs.Wrap(errors.Join(ErrTokenExchange, err), "client authentication")}
	}

	status, body, err := doForm(ctx, e.client, e.tokenURL, form, basicUser, basicPass)
	if err != nil {
		// Transport errors are transient; let the retry policy decide.
		return nil, &exchangeError{retryable: true, cause: coreerrs.Wrap(errors.Join(ErrTokenExchange, err), "token exchange")}
	}
	if status < 200 || status >= 300 {
		return nil, &exchangeError{
			retryable: status == http.StatusTooManyRequests || status >= 500,
			cause:     coreerrs.Wrapf(ErrTokenExchange, "unexpected status %d: %s", status, sanitizeErrorBody(body)),
		}
	}
	return parseTokenResponse(body)
}

// doForm POSTs form to tokenURL as application/x-www-form-urlencoded and returns
// the HTTP status and (size-limited) body. When basicUser is non-empty the
// request carries HTTP Basic credentials. A nil client uses [http.DefaultClient].
// A transport or read error is returned with status 0.
func doForm(
	ctx context.Context, client *http.Client, tokenURL string, form url.Values, basicUser, basicPass string,
) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if basicUser != "" {
		req.SetBasicAuth(url.QueryEscape(basicUser), url.QueryEscape(basicPass))
	}
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer drainAndClose(resp)

	body, err := readLimited(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, body, nil
}

// exchangeError classifies a token-exchange failure for the retry policy while
// preserving the underlying cause (which wraps [ErrTokenExchange]) for
// errors.Is / errors.As.
type exchangeError struct {
	retryable bool
	cause     error
}

func (e *exchangeError) Error() string { return e.cause.Error() }
func (e *exchangeError) Unwrap() error { return e.cause }

// TokenSource returns a self-refreshing [oauth2.TokenSource] that re-runs req
// through [Exchanger.Exchange] whenever the issued token expires, using ctx for
// each fetch. The subject token in req must stay valid across refreshes; when it
// expires the next refresh fails. Use it to attach an exchanged token to
// outbound calls the same way as the other helpers.
func (e *Exchanger) TokenSource(ctx context.Context, req ExchangeRequest) oauth2.TokenSource {
	return reuse(nil, &exchangeSource{exchanger: e, ctx: ctx, req: req}, e.earlyExpiry)
}

// exchangeSource adapts a fixed [ExchangeRequest] to [oauth2.TokenSource]. It
// pins ctx the way x/oauth2's own clientcredentials source does.
type exchangeSource struct {
	exchanger *Exchanger
	ctx       context.Context //nolint:containedctx // mirrors x/oauth2 TokenSource pinning ctx for background refresh
	req       ExchangeRequest
}

// Token performs a fresh exchange for the pinned request.
func (s *exchangeSource) Token() (*oauth2.Token, error) {
	return s.exchanger.Exchange(s.ctx, s.req)
}

// formValues builds the RFC 8693 request body for req.
func (e *Exchanger) formValues(req ExchangeRequest) url.Values {
	v := url.Values{}
	v.Set("grant_type", GrantTypeTokenExchange)
	v.Set("subject_token", req.SubjectToken)
	v.Set("subject_token_type", cmp.Or(req.SubjectTokenType, TokenTypeAccessToken))
	if req.ActorToken != "" {
		v.Set("actor_token", req.ActorToken)
		v.Set("actor_token_type", cmp.Or(req.ActorTokenType, TokenTypeAccessToken))
	}
	if req.Resource != "" {
		v.Set("resource", req.Resource)
	}
	if req.Audience != "" {
		v.Set("audience", req.Audience)
	}
	if len(req.Scopes) > 0 {
		v.Set("scope", strings.Join(req.Scopes, " "))
	}
	if req.RequestedTokenType != "" {
		v.Set("requested_token_type", req.RequestedTokenType)
	}
	for key, vals := range e.endpointParams {
		for _, val := range vals {
			v.Add(key, val)
		}
	}
	return v
}

// tokenExchangeResponse is the RFC 8693 §2.2.1 success response.
type tokenExchangeResponse struct {
	AccessToken     string      `json:"access_token"`
	IssuedTokenType string      `json:"issued_token_type"`
	TokenType       string      `json:"token_type"`
	ExpiresIn       json.Number `json:"expires_in"`
	Scope           string      `json:"scope"`
	RefreshToken    string      `json:"refresh_token"`
}

// parseTokenResponse decodes a token-exchange success body into an
// [oauth2.Token], stamping the relative expiry and stashing the issued token
// type and scope as Extra fields.
//
// Two RFC edge cases are preserved verbatim rather than normalized:
//   - A response without expires_in leaves Token.Expiry zero, which x/oauth2
//     treats as never-expiring — a self-refreshing source built on it will not
//     refresh. Callers relying on rotation should ensure the IdP returns expires_in.
//   - token_type "N_A" (RFC 8693, when the issued token is not an access token)
//     is kept as-is. Such a token is not a bearer credential; do not attach it
//     with Token.SetAuthHeader, which would emit "N_A ...".
func parseTokenResponse(body []byte) (*oauth2.Token, error) {
	var tr tokenExchangeResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, coreerrs.Wrap(errors.Join(ErrTokenExchange, err), "decode token-exchange response")
	}
	if tr.AccessToken == "" {
		return nil, coreerrs.Wrap(ErrTokenExchange, "response missing access_token")
	}
	tok := &oauth2.Token{
		AccessToken:  tr.AccessToken,
		TokenType:    tr.TokenType,
		RefreshToken: tr.RefreshToken,
	}
	if secs, err := tr.ExpiresIn.Int64(); err == nil && secs > 0 {
		tok.Expiry = time.Now().Add(time.Duration(secs) * time.Second)
	}
	extra := map[string]any{}
	if tr.IssuedTokenType != "" {
		extra["issued_token_type"] = tr.IssuedTokenType
	}
	if tr.Scope != "" {
		extra["scope"] = tr.Scope
	}
	if len(extra) > 0 {
		tok = tok.WithExtra(extra)
	}
	return tok, nil
}

// readLimited reads at most [maxExchangeResponseSize] bytes from r, returning a
// copy detached from the pooled buffer.
func readLimited(r io.Reader) ([]byte, error) {
	buf := coreio.GetBuffer()
	defer coreio.PutBuffer(buf)
	if _, err := buf.ReadFrom(io.LimitReader(r, maxExchangeResponseSize)); err != nil {
		return nil, err
	}
	return bytes.Clone(buf.Bytes()), nil
}

// drainAndClose drains and closes resp.Body so the connection can be reused.
func drainAndClose(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxExchangeResponseSize))
	_ = resp.Body.Close()
}

// sanitizeErrorBody truncates a raw response body to a safe length for error
// messages.
func sanitizeErrorBody(b []byte) string {
	if len(b) <= maxErrorBodyLen {
		return string(b)
	}
	return string(b[:maxErrorBodyLen]) + "...(truncated)"
}
