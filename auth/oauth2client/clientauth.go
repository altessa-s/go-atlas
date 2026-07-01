// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client

import (
	"context"
	"errors"
	"net/url"
	"time"

	"github.com/altessa-s/go-atlas/auth/jwt"

	"golang.org/x/oauth2"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// ClientAssertionTypeJWTBearer is the client_assertion_type value for a JWT
// bearer client assertion (RFC 7523 §2.2).
const ClientAssertionTypeJWTBearer = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer" // #nosec G101 -- RFC 7523 client_assertion_type URI, not a credential

// DefaultClientAssertionLifetime is how long a minted client assertion is valid.
// Assertions are short-lived and minted fresh per token request.
const DefaultClientAssertionLifetime = 5 * time.Minute

// algHS256 is the HMAC algorithm used for client_secret_jwt assertions. It is
// not in auth/jwt's asymmetric default allow-list, so the assertion signer is
// built with it explicitly.
const algHS256 = jwt.Algorithm("HS256")

// ErrClientAssertion wraps a failure to build or sign a client-authentication
// JWT assertion.
var ErrClientAssertion = errors.New("auth/oauth2client: client assertion failed")

// ClientAuthenticator authenticates the client to the token endpoint by
// producing the form parameters to add to a token request. It is the
// private_key_jwt / client_secret_jwt seam (RFC 7523): implementations mint a
// fresh, short-lived assertion per call, so the returned parameters must not be
// cached. audience is the token endpoint URL.
//
// When a [ClientAuthenticator] is set via [WithClientAuth], it supersedes the
// [WithAuthStyle] secret-based authentication. It applies to [ClientCredentials]
// and [Exchanger]; [Refresh] and [AuthCode] continue to use the secret path.
type ClientAuthenticator interface {
	AuthParams(ctx context.Context, audience string) (url.Values, error)
}

// PrivateKeyJWT authenticates with a JWT assertion signed by the client's
// private key (RFC 7523 private_key_jwt) — the FAPI / enterprise-IdP posture
// where no shared secret is presented. clientID is the OAuth2 client; key is the
// asymmetric [github.com/altessa-s/go-atlas/auth/jwt.SigningKey] whose KeyID
// (the assertion's kid header) must match the public key registered with the
// IdP. The key's algorithm may be any asymmetric JWA the IdP expects.
func PrivateKeyJWT(clientID string, key jwt.SigningKey, opts ...AssertionOption) (ClientAuthenticator, error) {
	if clientID == "" {
		return nil, coreerrs.Wrap(ErrClientAssertion, "client id is required")
	}
	if key.KeyID == "" {
		return nil, coreerrs.Wrap(ErrClientAssertion, "signing key id is required")
	}
	return newAssertionAuth(clientID, key, opts...), nil
}

// ClientSecretJWT authenticates with a JWT assertion HMAC-signed with the client
// secret (RFC 7523 client_secret_jwt) — a stronger alternative to sending the
// secret directly, as the secret never leaves the client. The assertion is
// signed with HS256 and carries the client id as its kid header.
func ClientSecretJWT(clientID, clientSecret string, opts ...AssertionOption) (ClientAuthenticator, error) {
	if clientID == "" || clientSecret == "" {
		return nil, coreerrs.Wrap(ErrClientAssertion, "client id and secret are required")
	}
	key := jwt.SigningKey{KeyID: clientID, Algorithm: algHS256, Key: []byte(clientSecret)}
	return newAssertionAuth(clientID, key, opts...), nil
}

// applyClientAuth adds client-authentication parameters to form for a request to
// audience (the token or revocation endpoint) and returns the HTTP Basic
// credentials the caller must present, empty when the body already carries the
// credentials or a JWT assertion. A non-nil ca (a [ClientAuthenticator])
// supersedes the secret; otherwise authStyle selects header (Basic) vs. body.
func applyClientAuth(
	ctx context.Context, form url.Values, ca ClientAuthenticator,
	authStyle oauth2.AuthStyle, clientID, clientSecret, audience string,
) (basicUser, basicPass string, err error) {
	if ca != nil {
		params, err := ca.AuthParams(ctx, audience)
		if err != nil {
			return "", "", err
		}
		for key, vals := range params {
			for _, val := range vals {
				form.Set(key, val)
			}
		}
		return "", "", nil
	}
	if authStyle == oauth2.AuthStyleInParams {
		if clientID != "" {
			form.Set("client_id", clientID)
			if clientSecret != "" {
				form.Set("client_secret", clientSecret)
			}
		}
		return "", "", nil
	}
	if clientID != "" {
		return clientID, clientSecret, nil
	}
	return "", "", nil
}

// assertionAuth mints RFC 7523 client-assertion JWTs.
type assertionAuth struct {
	signer   *jwt.Signer
	key      jwt.SigningKey
	clientID string
	lifetime time.Duration
	audience string
}

// newAssertionAuth builds an [assertionAuth] whose signer accepts exactly the
// key's algorithm.
func newAssertionAuth(clientID string, key jwt.SigningKey, opts ...AssertionOption) *assertionAuth {
	cfg := newAssertionConfig(opts...)
	return &assertionAuth{
		signer:   jwt.NewSigner(jwt.WithAllowedAlgorithms(key.Algorithm)),
		key:      key,
		clientID: clientID,
		lifetime: cfg.lifetime,
		audience: cfg.audience,
	}
}

// AuthParams mints a fresh assertion (iss = sub = client id, aud = the token
// endpoint, unique jti, short exp) and returns the client_assertion form
// parameters.
func (a *assertionAuth) AuthParams(_ context.Context, audience string) (url.Values, error) {
	aud := a.audience
	if aud == "" {
		aud = audience
	}
	claims, err := a.signer.NewClaims(a.clientID, a.lifetime)
	if err != nil {
		return nil, coreerrs.Wrap(errors.Join(ErrClientAssertion, err), "build client assertion")
	}
	claims["iss"] = a.clientID
	claims["aud"] = aud

	raw, err := a.signer.Sign(a.key, claims)
	if err != nil {
		return nil, coreerrs.Wrap(errors.Join(ErrClientAssertion, err), "sign client assertion")
	}

	v := url.Values{}
	v.Set("client_id", a.clientID)
	v.Set("client_assertion_type", ClientAssertionTypeJWTBearer)
	v.Set("client_assertion", raw)
	return v, nil
}

// assertionConfig carries the tunables for a client assertion.
type assertionConfig struct {
	lifetime time.Duration
	audience string
}

// newAssertionConfig applies opts over the defaults.
func newAssertionConfig(opts ...AssertionOption) assertionConfig {
	c := assertionConfig{lifetime: DefaultClientAssertionLifetime}
	for _, o := range opts {
		o(&c)
	}
	return c
}

// AssertionOption configures the client assertion minted by [PrivateKeyJWT] and
// [ClientSecretJWT].
type AssertionOption func(*assertionConfig)

// WithAssertionLifetime overrides the assertion validity window
// ([DefaultClientAssertionLifetime]). A non-positive value is ignored.
func WithAssertionLifetime(d time.Duration) AssertionOption {
	return func(c *assertionConfig) {
		if d > 0 {
			c.lifetime = d
		}
	}
}

// WithAssertionAudience overrides the assertion aud claim. By default it is the
// token endpoint URL; set this when the IdP requires its issuer identifier
// instead.
func WithAssertionAudience(aud string) AssertionOption {
	return func(c *assertionConfig) {
		if aud != "" {
			c.audience = aud
		}
	}
}
