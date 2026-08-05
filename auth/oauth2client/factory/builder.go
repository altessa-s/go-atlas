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

	"github.com/altessa-s/go-atlas/auth/jwt"
	"github.com/altessa-s/go-atlas/auth/oauth2client"
	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/core/collections/slices"

	"golang.org/x/oauth2"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	gojwt "github.com/golang-jwt/jwt/v5"
)

// estimatedOptionsCount pre-sizes the option slice: scopes, auth style,
// endpoint params, HTTP client, and three retry options.
const estimatedOptionsCount = 7

// Builder assembles client-side OAuth2 token acquisition from configuration and
// injected dependencies. Build produces a self-refreshing client_credentials
// [oauth2.TokenSource]; BuildExchanger produces an RFC 8693 [oauth2client.Exchanger].
type Builder struct {
	corefactory.Base
	cfg  *config.OAuth2Client
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
func New(cfg *config.OAuth2Client) *Builder {
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
	tokenURL, opts, err := b.resolve()
	if err != nil {
		return nil, err
	}
	return oauth2client.ClientCredentials(ctx, tokenURL, b.cfg.ClientId, b.cfg.ClientSecret.Expose(), opts...), nil
}

// BuildExchanger assembles an RFC 8693 [oauth2client.Exchanger] from the same
// configuration. The subject token is supplied per call at exchange time.
func (b *Builder) BuildExchanger(_ context.Context) (*oauth2client.Exchanger, error) {
	tokenURL, opts, err := b.resolve()
	if err != nil {
		return nil, err
	}
	return oauth2client.NewExchanger(tokenURL, b.cfg.ClientId, b.cfg.ClientSecret.Expose(), opts...), nil
}

// resolve validates the builder, resolves the token endpoint, and translates
// config into oauth2client options.
func (b *Builder) resolve() (string, []oauth2client.Option, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return "", nil, err
	}
	if b.cfg == nil {
		return "", nil, b.Errorf("configuration is required")
	}
	tokenURL, err := b.resolveTokenURL()
	if err != nil {
		return "", nil, err
	}
	opts := b.options()
	clientAuthOpt, err := b.clientAuthOption()
	if err != nil {
		return "", nil, err
	}
	opts = slices.AppendNonNil(opts, clientAuthOpt)
	return tokenURL, opts, nil
}

// resolveTokenURL returns the literal token endpoint or resolves it from the
// injected discovery source.
func (b *Builder) resolveTokenURL() (string, error) {
	if b.cfg.TokenUrl != "" {
		return b.cfg.TokenUrl, nil
	}
	if b.tokenEndpoint == nil {
		return "", b.Errorf("discoveryUrl is set but no token endpoint source was injected; call UseTokenEndpointSource")
	}
	tokenURL := b.tokenEndpoint.TokenEndpoint()
	if tokenURL == "" {
		return "", oauth2client.ErrNoTokenEndpoint
	}
	return tokenURL, nil
}

// options translates config into the oauth2client option slice.
func (b *Builder) options() []oauth2client.Option {
	cfg := b.cfg
	opts := make([]oauth2client.Option, 0, estimatedOptionsCount)

	opts = slices.AppendIf(opts, len(cfg.Scopes) > 0, oauth2client.WithScopes(cfg.Scopes...))
	opts = append(opts, oauth2client.WithAuthStyle(authStyle(cfg.AuthStyle)))
	opts = slices.AppendIf(opts, len(cfg.EndpointParams) > 0,
		oauth2client.WithEndpointParams(endpointParams(cfg.EndpointParams)))
	opts = slices.AppendNonNil(opts, b.httpClientOption())
	opts = slices.AppendIf(opts, b.logger != nil, oauth2client.WithLogger(b.logger))
	opts = slices.AppendIf(opts, b.metrics != nil, oauth2client.WithMetrics(b.metrics))
	opts = slices.AppendIf(opts, cfg.EarlyExpiry > 0, oauth2client.WithEarlyExpiry(cfg.EarlyExpiry))
	opts = slices.AppendIfFunc(opts, cfg.IsRetryConfigured() && cfg.Retry.Attempts > 0, func() []oauth2client.Option {
		return []oauth2client.Option{
			oauth2client.WithRetryAttempts(cfg.Retry.Attempts),
			oauth2client.WithRetryBaseDelay(cfg.Retry.BaseDelay),
			oauth2client.WithRetryMaxDelay(cfg.Retry.MaxDelay),
		}
	})
	return opts
}

// httpClientOption returns the injected-client option, or nil when none was set.
func (b *Builder) httpClientOption() oauth2client.Option {
	if b.httpClient == nil {
		return nil
	}
	return oauth2client.WithHTTPClient(b.httpClient)
}

// clientAuthOption returns the client-authenticator option. A programmatically
// injected authenticator (via [Builder.UseClientAuth]) wins; otherwise one is
// built from the config clientAuth block. It returns nil when neither is set.
func (b *Builder) clientAuthOption() (oauth2client.Option, error) {
	ca := b.clientAuth
	if ca == nil && b.cfg.ClientAuth != nil {
		built, err := b.buildConfigClientAuth(b.cfg.ClientAuth)
		if err != nil {
			return nil, err
		}
		ca = built
	}
	if ca == nil {
		return nil, nil //nolint:nilnil // no client auth configured: nil option, no error
	}
	return oauth2client.WithClientAuth(ca), nil
}

// buildConfigClientAuth constructs a [oauth2client.ClientAuthenticator] from the
// config clientAuth block, loading the signing key for private_key_jwt.
func (b *Builder) buildConfigClientAuth(ca *config.OAuth2ClientAuth) (oauth2client.ClientAuthenticator, error) {
	var opts []oauth2client.AssertionOption
	opts = slices.AppendIf(opts, ca.AssertionLifetime > 0, oauth2client.WithAssertionLifetime(ca.AssertionLifetime))
	opts = slices.AppendIf(opts, ca.AssertionAudience != "", oauth2client.WithAssertionAudience(ca.AssertionAudience))
	switch ca.Method {
	case config.OAuth2ClientAuthClientSecretJWT:
		return oauth2client.ClientSecretJWT(b.cfg.ClientId, b.cfg.ClientSecret.Expose(), opts...)
	case config.OAuth2ClientAuthPrivateKeyJWT:
		key, err := signingKeyFromConfig(ca)
		if err != nil {
			return nil, b.WrapError(err, "build assertion signing key")
		}
		return oauth2client.PrivateKeyJWT(b.cfg.ClientId, key, opts...)
	default:
		return nil, b.Errorf("unknown clientAuth method %q", ca.Method)
	}
}

// signingKeyFromConfig parses the PEM private key into a [jwt.SigningKey],
// selecting the parser by algorithm family.
func signingKeyFromConfig(ca *config.OAuth2ClientAuth) (jwt.SigningKey, error) {
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
