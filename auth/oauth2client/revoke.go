// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"golang.org/x/oauth2"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// token_type_hint values for [Revoker.Revoke] (RFC 7009 §2.1).
const (
	TokenTypeHintAccessToken  = "access_token"
	TokenTypeHintRefreshToken = "refresh_token"
)

// Revoker revokes previously acquired tokens at the IdP's revocation endpoint
// (RFC 7009) — the symmetric counterpart to acquisition, letting a service
// proactively invalidate a token it no longer needs. It authenticates the client
// the same way as the token endpoint: the [WithAuthStyle] secret or a
// [WithClientAuth] JWT assertion. A Revoker is immutable and safe for concurrent
// use.
type Revoker struct {
	revocationURL string
	clientID      string
	clientSecret  string
	authStyle     oauth2.AuthStyle
	clientAuth    ClientAuthenticator
	client        *http.Client
}

// NewRevoker builds a [Revoker] for the IdP revocation endpoint at
// revocationURL. clientID and clientSecret authenticate this client (pass "" for
// both when a [WithClientAuth] assertion or a public client is used). The
// [WithAuthStyle], [WithClientAuth], and [WithHttpClient] options apply.
func NewRevoker(revocationURL, clientID, clientSecret string, opts ...Option) *Revoker {
	o := newOptions(opts...)
	return &Revoker{
		revocationURL: revocationURL,
		clientID:      clientID,
		clientSecret:  clientSecret,
		authStyle:     o.authStyle,
		clientAuth:    o.clientAuth,
		client:        o.httpClient,
	}
}

// revokeConfig carries per-call revocation options.
type revokeConfig struct {
	hint string
}

// RevokeOption configures a single [Revoker.Revoke] call.
type RevokeOption func(*revokeConfig)

// WithTokenTypeHint sets the token_type_hint parameter ([TokenTypeHintAccessToken]
// or [TokenTypeHintRefreshToken]), letting the IdP look the token up faster. It
// is advisory: the IdP must still revoke a token whose type differs from the hint.
func WithTokenTypeHint(hint string) RevokeOption {
	return func(c *revokeConfig) { c.hint = hint }
}

// Revoke revokes token at the revocation endpoint. Per RFC 7009 the IdP responds
// 200 even for an unknown or already-invalid token, so a nil return means the
// token is not (or no longer) valid — not that it was necessarily active. An
// empty token, a transport failure, or a non-2xx response returns an error
// wrapping [ErrRevocation].
func (r *Revoker) Revoke(ctx context.Context, token string, opts ...RevokeOption) error {
	if token == "" {
		return coreerrs.Wrap(ErrRevocation, "token is required")
	}
	var cfg revokeConfig
	for _, o := range opts {
		o(&cfg)
	}

	form := url.Values{}
	form.Set("token", token)
	if cfg.hint != "" {
		form.Set("token_type_hint", cfg.hint)
	}
	basicUser, basicPass, err := applyClientAuth(ctx, form, r.clientAuth, r.authStyle, r.clientID, r.clientSecret, r.revocationURL)
	if err != nil {
		return coreerrs.Wrap(errors.Join(ErrRevocation, err), "client authentication")
	}

	status, body, err := doForm(ctx, r.client, r.revocationURL, form, basicUser, basicPass)
	if err != nil {
		return coreerrs.Wrap(errors.Join(ErrRevocation, err), "revoke token")
	}
	if status < 200 || status >= 300 {
		return coreerrs.Wrapf(ErrRevocation, "unexpected status %d: %s", status, sanitizeErrorBody(body))
	}
	return nil
}
