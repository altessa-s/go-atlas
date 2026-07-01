// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client

import (
	"context"

	"golang.org/x/oauth2"
)

// TokenEndpointSource yields an IdP's token endpoint URL — typically from an
// OIDC discovery document. [github.com/altessa-s/go-atlas/auth/oidc.Provider]
// satisfies it through its TokenEndpoint method, so a verified provider's
// discovered endpoint can seed the client-side flows instead of hardcoding the
// URL, keeping acquisition and verification pointed at the same IdP metadata.
// It is a narrow consumer-side interface: this package does not import oidc.
type TokenEndpointSource interface {
	TokenEndpoint() string
}

// ClientCredentialsFromDiscovery is [ClientCredentials] with the token endpoint
// resolved from src (an OIDC discovery document) rather than a literal URL. It
// returns [ErrNoTokenEndpoint] when src has not discovered a token endpoint —
// for example when discovery has not yet completed or the IdP omits it.
func ClientCredentialsFromDiscovery(
	ctx context.Context, src TokenEndpointSource, clientID, clientSecret string, opts ...Option,
) (oauth2.TokenSource, error) {
	tokenURL := src.TokenEndpoint()
	if tokenURL == "" {
		return nil, ErrNoTokenEndpoint
	}
	return ClientCredentials(ctx, tokenURL, clientID, clientSecret, opts...), nil
}

// NewExchangerFromDiscovery is [NewExchanger] with the token endpoint resolved
// from src. It returns [ErrNoTokenEndpoint] when src has not discovered one.
func NewExchangerFromDiscovery(
	src TokenEndpointSource, clientID, clientSecret string, opts ...Option,
) (*Exchanger, error) {
	tokenURL := src.TokenEndpoint()
	if tokenURL == "" {
		return nil, ErrNoTokenEndpoint
	}
	return NewExchanger(tokenURL, clientID, clientSecret, opts...), nil
}
