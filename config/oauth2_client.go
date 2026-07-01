// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"time"

	"github.com/go-ozzo/ozzo-validation/v4/is"

	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Client-authentication methods for [OAuth2ClientAuth.Method] (RFC 7523).
const (
	OAuth2ClientAuthPrivateKeyJWT   = "private_key_jwt"
	OAuth2ClientAuthClientSecretJWT = "client_secret_jwt"
)

// Default values for OAuth2Client configuration.
const (
	defaultOAuth2ClientAuthStyle      = "auto"
	defaultOAuth2ClientRetryAttempts  = 0
	defaultOAuth2ClientRetryBaseDelay = 200 * time.Millisecond
	defaultOAuth2ClientRetryMaxDelay  = 5 * time.Second
)

// OAuth2Client configures client-side OAuth2 token acquisition from an external
// identity provider (the acquisition counterpart to the inbound-facing OIDC
// config). It drives the auth/oauth2client factory, which produces a
// self-refreshing client_credentials token source (or a token exchanger) for
// outbound, service-to-service calls.
type OAuth2Client struct {
	// TokenUrl is the IdP token endpoint. Provide either this or DiscoveryUrl.
	TokenUrl string `yaml:"tokenUrl"`

	// DiscoveryUrl is an OIDC discovery document; the token endpoint is resolved
	// from it at build time. Provide either this or TokenUrl.
	DiscoveryUrl string `yaml:"discoveryUrl"`

	// ClientId is the OAuth2 client identifier authenticating this service.
	ClientId string `yaml:"clientId"`

	// ClientSecret is the OAuth2 client secret. Leave empty for a public client.
	ClientSecret Secret `yaml:"clientSecret"`

	// Scopes are the OAuth2 scopes requested from the IdP.
	Scopes []string `yaml:"scopes"`

	// AuthStyle selects how client credentials are presented: "auto" (detect),
	// "header" (HTTP Basic), or "params" (request body).
	AuthStyle string `yaml:"authStyle" default:"auto"`

	// EndpointParams are extra, non-standard token-request parameters (for
	// example Auth0's "audience").
	EndpointParams map[string]string `yaml:"endpointParams"`

	// EarlyExpiry refreshes tokens this long before their exp to absorb clock
	// skew and in-flight latency. Zero uses the x/oauth2 default (~10s). Applies
	// to client_credentials and token exchange.
	EarlyExpiry time.Duration `yaml:"earlyExpiry"`

	// Retry configures retrying of token exchanges. Omitted disables retrying.
	Retry *OAuth2ClientRetry `yaml:"retry" default:"-"`

	// ClientAuth configures JWT-assertion client authentication (RFC 7523),
	// superseding the plain clientSecret. Omitted uses the secret / authStyle.
	ClientAuth *OAuth2ClientAuth `yaml:"clientAuth" default:"-"`
}

// DefaultOAuth2Client returns an OAuth2Client configuration with default values.
// TokenUrl, DiscoveryUrl, and ClientId are left as zero values since they are
// deployment-specific.
func DefaultOAuth2Client() OAuth2Client {
	return OAuth2Client{
		AuthStyle: defaultOAuth2ClientAuthStyle,
	}
}

// Validate performs validation on the OAuth2Client configuration. Exactly one of
// TokenUrl or DiscoveryUrl must be a valid URL, ClientId is required, and
// AuthStyle must be one of the supported styles.
func (c *OAuth2Client) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.TokenUrl,
			validation.When(c.DiscoveryUrl == "", validation.Required),
			validation.When(c.TokenUrl != "", is.URL),
		),
		validation.Field(&c.DiscoveryUrl,
			validation.When(c.TokenUrl == "", validation.Required),
			validation.When(c.DiscoveryUrl != "", is.URL),
		),
		validation.Field(&c.ClientId, validation.Required),
		validation.Field(&c.AuthStyle, validation.In("auto", "header", "params")),
		validation.Field(&c.EarlyExpiry, validation.Min(time.Duration(0))),
		validation.Field(&c.Scopes, validation.Each(validation.Required)),
		validation.Field(&c.Retry, validation.NilOrNotEmpty),
		validation.Field(&c.ClientAuth, validation.NilOrNotEmpty),
	)
}

// IsClientAuthConfigured reports whether JWT-assertion client auth is present.
func (c *OAuth2Client) IsClientAuthConfigured() bool {
	return c.ClientAuth != nil
}

// IsDiscovery reports whether the token endpoint is resolved via OIDC discovery.
func (c *OAuth2Client) IsDiscovery() bool {
	return c.TokenUrl == "" && c.DiscoveryUrl != ""
}

// IsRetryConfigured reports whether retry settings are present.
func (c *OAuth2Client) IsRetryConfigured() bool {
	return c.Retry != nil
}

// OAuth2ClientRetry configures retrying of token exchanges with exponential
// backoff. Attempts is the number of retries after the first try; zero disables
// retrying.
type OAuth2ClientRetry struct {
	// Attempts is the number of retries after the first attempt (0 disables).
	Attempts int `yaml:"attempts" default:"0"`

	// BaseDelay is the backoff before the first retry.
	BaseDelay time.Duration `yaml:"baseDelay" default:"200ms"`

	// MaxDelay caps the exponential backoff between retries.
	MaxDelay time.Duration `yaml:"maxDelay" default:"5s"`
}

// DefaultOAuth2ClientRetry returns retry settings with default values.
func DefaultOAuth2ClientRetry() OAuth2ClientRetry {
	return OAuth2ClientRetry{
		Attempts:  defaultOAuth2ClientRetryAttempts,
		BaseDelay: defaultOAuth2ClientRetryBaseDelay,
		MaxDelay:  defaultOAuth2ClientRetryMaxDelay,
	}
}

// Validate performs validation on the OAuth2ClientRetry configuration.
func (r *OAuth2ClientRetry) Validate() error {
	return ValidateStruct(r,
		validation.Field(&r.Attempts, validation.Min(0)),
		validation.Field(&r.BaseDelay, validation.Min(time.Duration(0))),
		validation.Field(&r.MaxDelay, validation.Min(time.Duration(0))),
	)
}

// OAuth2ClientAuth configures JWT-assertion client authentication (RFC 7523):
// private_key_jwt (a signed assertion) or client_secret_jwt (HMAC over the
// clientSecret). It supersedes the plain clientSecret / authStyle for the
// client_credentials and token-exchange flows.
type OAuth2ClientAuth struct {
	// Method is "private_key_jwt" or "client_secret_jwt".
	Method string `yaml:"method"`

	// PrivateKey is the PEM-encoded signing key (private_key_jwt only). It is a
	// Secret so it can be sourced from a file/env/vault reference.
	PrivateKey Secret `yaml:"privateKey"`

	// KeyId is the assertion's kid header, matching the public key registered
	// with the IdP (private_key_jwt only).
	KeyId string `yaml:"keyId"`

	// Algorithm is the asymmetric JWA used to sign the assertion (private_key_jwt).
	Algorithm string `yaml:"algorithm" default:"RS256"`

	// AssertionLifetime is the assertion validity window. Zero uses the default (5m).
	AssertionLifetime time.Duration `yaml:"assertionLifetime"`

	// AssertionAudience overrides the assertion aud claim (default: token endpoint).
	AssertionAudience string `yaml:"assertionAudience"`
}

// Validate performs validation on the OAuth2ClientAuth configuration.
func (a *OAuth2ClientAuth) Validate() error {
	return ValidateStruct(a,
		validation.Field(&a.Method, validation.Required,
			validation.In(OAuth2ClientAuthPrivateKeyJWT, OAuth2ClientAuthClientSecretJWT)),
		validation.Field(&a.PrivateKey, validation.When(a.Method == OAuth2ClientAuthPrivateKeyJWT, validation.Required)),
		validation.Field(&a.KeyId, validation.When(a.Method == OAuth2ClientAuthPrivateKeyJWT, validation.Required)),
		validation.Field(&a.Algorithm, validation.When(a.Method == OAuth2ClientAuthPrivateKeyJWT,
			validation.In("EdDSA", "ES256", "ES384", "ES512", "RS256", "RS384", "RS512", "PS256", "PS384", "PS512"))),
		validation.Field(&a.AssertionLifetime, validation.Min(time.Duration(0))),
	)
}
