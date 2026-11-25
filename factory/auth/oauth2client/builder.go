// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"crypto"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/altessa-s/go-atlas-core/collections/slices"
	"github.com/altessa-s/go-atlas/auth/jwt"
	"github.com/altessa-s/go-atlas/auth/oauth2client"

	"golang.org/x/oauth2"

	authconfig "github.com/altessa-s/go-atlas/config/auth"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	gojwt "github.com/golang-jwt/jwt/v5"
)

// Builder assembles client-side OAuth2 token acquisition from configuration and
// injected dependencies. Build produces a self-refreshing client_credentials
// [oauth2.TokenSource]; BuildExchanger produces an RFC 8693 [oauth2client.Exchanger].
type Builder struct {
	corefactory.Base
	cfg  *authconfig.OAuth2Client
	errs []error

	// Dependencies (set via Use*).
	tokenEndpoint oauth2client.TokenEndpointSource
	httpClient    *http.Client
	logger        *slog.Logger
	metrics       *oauth2client.Metrics
	clientAuth    oauth2client.ClientAuthenticator
}

// New creates a [Builder] for the given config. A nil cfg is accepted; the
// error surfaces at build time.
func New(cfg *authconfig.OAuth2Client) *Builder {
	return &Builder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// UseTokenEndpointSource injects the OIDC discovery source used to resolve the
// token endpoint when the config sets discoveryUrl instead of tokenUrl.
// [github.com/altessa-s/go-atlas/auth/oidc.Provider] satisfies it.
func (b *Builder) UseTokenEndpointSource(src oauth2client.TokenEndpointSource) *Builder {
	b.tokenEndpoint = src
	return b
}

// UseHTTPClient injects the HTTP client the token-endpoint requests are issued
// with (for timeouts, mTLS, or tracing). Optional.
func (b *Builder) UseHTTPClient(client *http.Client) *Builder {
	b.httpClient = client
	return b
}

// UseLogger injects the logger used to record token-fetch failures and retry
// attempts. Optional; nil disables logging.
func (b *Builder) UseLogger(logger *slog.Logger) *Builder {
	b.logger = logger
	return b
}

// UseMetrics injects the metrics sink recording fetch counts, latency, and
// retries. Build one with [oauth2client.NewMetrics]. Optional.
func (b *Builder) UseMetrics(m *oauth2client.Metrics) *Builder {
	b.metrics = m
	return b
}

// UseClientAuth injects a JWT-assertion client authenticator (build one with
// [oauth2client.PrivateKeyJWT] or [oauth2client.ClientSecretJWT]), superseding
// the secret-based auth for the client_credentials and exchanger flows. Optional.
func (b *Builder) UseClientAuth(a oauth2client.ClientAuthenticator) *Builder {
	b.clientAuth = a
	return b
}

// Build assembles a self-refreshing client_credentials [oauth2.TokenSource].
func (b *Builder) Build(ctx context.Context) (oauth2.TokenSource, error) {
	tokenURL, auth, err := b.resolve()
	if err != nil {
		return nil, err
	}
	opts := []oauth2client.ClientCredentialsOption{
		oauth2client.WithClientCredentialsAuthStyle(authStyle(b.cfg.AuthStyle)),
		oauth2client.WithClientCredentialsHTTPClient(b.httpClient),
		oauth2client.WithClientCredentialsMetrics(b.metrics),
		oauth2client.WithClientCredentialsLogger(b.logger),
		oauth2client.WithClientCredentialsEarlyExpiry(b.cfg.EarlyExpiry),
		oauth2client.WithClientCredentialsClientAuth(auth),
		oauth2client.WithClientCredentialsScopes(b.cfg.Scopes...),
	}
	if len(b.cfg.EndpointParams) > 0 {
		opts = append(opts, oauth2client.WithClientCredentialsEndpointParams(endpointParams(b.cfg.EndpointParams)))
	}
	if b.cfg.AllowInsecureHTTP {
		opts = append(opts, oauth2client.WithClientCredentialsInsecureHTTP())
	}
	return oauth2client.ClientCredentials(ctx, tokenURL, b.cfg.ClientId, b.cfg.ClientSecret.Expose(), opts...)
}

// BuildExchanger assembles an RFC 8693 [oauth2client.Exchanger] from the same
// configuration. The subject token is supplied per call at exchange time.
func (b *Builder) BuildExchanger(_ context.Context) (*oauth2client.Exchanger, error) {
	tokenURL, auth, err := b.resolve()
	if err != nil {
		return nil, err
	}
	opts := []oauth2client.ExchangerOption{
		oauth2client.WithExchangerAuthStyle(authStyle(b.cfg.AuthStyle)),
		oauth2client.WithExchangerHTTPClient(b.httpClient),
		oauth2client.WithExchangerMetrics(b.metrics),
		oauth2client.WithExchangerLogger(b.logger),
		oauth2client.WithExchangerEarlyExpiry(b.cfg.EarlyExpiry),
		oauth2client.WithExchangerClientAuth(auth),
	}
	if len(b.cfg.EndpointParams) > 0 {
		opts = append(opts, oauth2client.WithExchangerEndpointParams(endpointParams(b.cfg.EndpointParams)))
	}
	if b.cfg.AllowInsecureHTTP {
		opts = append(opts, oauth2client.WithExchangerInsecureHTTP())
	}
	if b.cfg.IsRetryConfigured() && b.cfg.Retry.Attempts > 0 {
		opts = append(opts,
			oauth2client.WithExchangerRetryAttempts(b.cfg.Retry.Attempts),
			oauth2client.WithExchangerRetryBaseDelay(b.cfg.Retry.BaseDelay),
			oauth2client.WithExchangerRetryMaxDelay(b.cfg.Retry.MaxDelay),
		)
	}
	return oauth2client.NewExchanger(tokenURL, b.cfg.ClientId, b.cfg.ClientSecret.Expose(), opts...)
}

// resolve validates the builder and resolves the token endpoint and client authentication.
func (b *Builder) resolve() (string, oauth2client.ClientAuthenticator, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return "", nil, err
	}
	if b.cfg == nil {
		return "", nil, corefactory.ErrConfigRequired
	}
	tokenURL, err := b.resolveTokenURL()
	if err != nil {
		return "", nil, err
	}
	if b.clientAuth == nil && b.cfg.ClientAuth != nil {
		auth, err := b.buildConfigClientAuth(b.cfg.ClientAuth)
		return tokenURL, auth, err
	}
	return tokenURL, b.clientAuth, nil
}

// resolveTokenURL returns the literal token endpoint or resolves it from the
// injected discovery source. Either way the endpoint must pass
// [oauth2client.ValidateEndpointURL]: a discovered endpoint is checked like a
// configured one, since the client credentials are sent to it.
func (b *Builder) resolveTokenURL() (string, error) {
	tokenURL := b.cfg.TokenUrl
	if tokenURL == "" {
		if b.tokenEndpoint == nil {
			return "", fmt.Errorf("discoveryUrl is set but no token endpoint source was injected; call UseTokenEndpointSource")
		}
		tokenURL = b.tokenEndpoint.TokenEndpoint()
		if tokenURL == "" {
			return "", oauth2client.ErrNoTokenEndpoint
		}
	}
	if err := oauth2client.ValidateEndpointURL(tokenURL, b.cfg.AllowInsecureHTTP); err != nil {
		return "", err
	}
	return tokenURL, nil
}

// buildConfigClientAuth constructs a [oauth2client.ClientAuthenticator] from the
// config clientAuth block, loading the signing key for private_key_jwt.
func (b *Builder) buildConfigClientAuth(ca *authconfig.OAuth2ClientAuth) (oauth2client.ClientAuthenticator, error) {
	var opts []oauth2client.AssertionOption
	opts = slices.AppendIf(opts, ca.AssertionLifetime > 0, oauth2client.WithAssertionLifetime(ca.AssertionLifetime))
	opts = slices.AppendIf(opts, ca.AssertionAudience != "", oauth2client.WithAssertionAudience(ca.AssertionAudience))
	switch ca.Method {
	case authconfig.OAuth2ClientAuthClientSecretJWT:
		return oauth2client.ClientSecretJWT(b.cfg.ClientId, b.cfg.ClientSecret.Expose(), opts...)
	case authconfig.OAuth2ClientAuthPrivateKeyJWT:
		key, err := signingKeyFromConfig(ca)
		if err != nil {
			return nil, b.WrapError(err, "build assertion signing key")
		}
		return oauth2client.PrivateKeyJWT(b.cfg.ClientId, key, opts...)
	default:
		return nil, fmt.Errorf("unknown clientAuth method %q", ca.Method)
	}
}

// signingKeyFromConfig parses the PEM private key into a [jwt.SigningKey],
// selecting the parser by algorithm family.
func signingKeyFromConfig(ca *authconfig.OAuth2ClientAuth) (jwt.SigningKey, error) {
	pemKey := []byte(ca.PrivateKey.Expose())
	var (
		key crypto.PrivateKey
		err error
	)
	switch {
	case strings.HasPrefix(ca.Algorithm, "RS"), strings.HasPrefix(ca.Algorithm, "PS"):
		key, err = gojwt.ParseRSAPrivateKeyFromPEM(pemKey)
	case strings.HasPrefix(ca.Algorithm, "ES"):
		key, err = gojwt.ParseECPrivateKeyFromPEM(pemKey)
	case ca.Algorithm == "EdDSA":
		key, err = gojwt.ParseEdPrivateKeyFromPEM(pemKey)
	default:
		return jwt.SigningKey{}, fmt.Errorf("unsupported assertion algorithm %q", ca.Algorithm)
	}
	if err != nil {
		return jwt.SigningKey{}, fmt.Errorf("parse assertion private key: %w", err)
	}
	return jwt.SigningKey{KeyID: ca.KeyId, Algorithm: jwt.Algorithm(ca.Algorithm), Key: key}, nil
}

// authStyle maps the config string to an [oauth2.AuthStyle]. Unknown values fall
// back to auto-detect (Validate already restricts the set).
func authStyle(s string) oauth2.AuthStyle {
	switch s {
	case "header":
		return oauth2.AuthStyleInHeader
	case "params":
		return oauth2.AuthStyleInParams
	default:
		return oauth2.AuthStyleAutoDetect
	}
}

// endpointParams converts the config map to url.Values.
func endpointParams(m map[string]string) url.Values {
	v := make(url.Values, len(m))
	for key, val := range m {
		v.Set(key, val)
	}
	return v
}
