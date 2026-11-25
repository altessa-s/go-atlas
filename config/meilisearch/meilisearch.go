// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package meilisearchconfig

import (
	"net/url"
	"strings"
	"time"

	"github.com/go-ozzo/ozzo-validation/v4/is"

	"github.com/altessa-s/go-atlas-core/types/redacted"

	tlsconfig "github.com/altessa-s/go-atlas/config/tls"
	validationconfig "github.com/altessa-s/go-nomos/validation"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for Meilisearch configuration.
const (
	defaultMeilisearchHost    = "http://localhost:7700"
	defaultMeilisearchTimeout = 30 * time.Second
)

// ErrInvalidHost reports a malformed Meilisearch base URL.
var ErrInvalidHost = validation.NewError("meilisearch_invalid_host", "must be a valid HTTP or HTTPS base URL without a query or fragment")

// ErrTLSRequiresHTTPS reports a TLS configuration paired with an HTTP URL.
var ErrTLSRequiresHTTPS = validation.NewError("meilisearch_tls_requires_https", "must use HTTPS when TLS is configured")

// Config represents the configuration for connecting to a Meilisearch
// instance. It contains the server URL, optional API key, HTTP timeout,
// and optional TLS settings for HTTPS deployments.
//
// Example:
//
//	ms := &meilisearchconfig.Config{
//		Host:    "https://meilisearch.example.com",
//		APIKey:  "masterKey",
//		Timeout: 30 * time.Second,
//		TLS: &tlsconfig.Client{
//			CACerts:        []string{"/etc/ssl/internal-ca.pem"},
//			SkipVerifyMode: tlsconfig.SkipVerifyModeEnforce,
//		},
//	}
type Config struct {
	// Host is the HTTP or HTTPS Meilisearch base URL without a query or fragment.
	// Defaults to "http://localhost:7700".
	Host string `yaml:"host" default:"http://localhost:7700"`

	// APIKey is the master or admin API key sent in the Authorization
	// header. Leave empty to disable authentication — useful for local
	// development against an open instance.
	APIKey redacted.RedactedString `yaml:"apiKey"`

	// Timeout is the HTTP client timeout for Meilisearch requests.
	// Defaults to 30 seconds.
	Timeout time.Duration `yaml:"timeout" default:"30s"`

	// TLS configures the HTTPS transport and requires an HTTPS Host. The factory
	// builds an *http.Client with the resulting *tls.Config and threads
	// it into the Meilisearch SDK via WithHTTPClient. Leave nil for
	// plaintext HTTP (the SDK still works, but the connection is
	// unencrypted — only safe for localhost or internal-network
	// deployments).
	//
	// Honors the SkipVerifyMode safety guard: setting SkipVerify=true
	// without explicitly opting into a less-strict mode is refused at
	// build time.
	TLS *tlsconfig.Client `yaml:"tls" default:"-"`
}

// Default returns a [Config] configuration populated with
// default values.
func Default() Config {
	return Config{
		Host:    defaultMeilisearchHost,
		Timeout: defaultMeilisearchTimeout,
	}
}

// Validate performs validation on the [Config] configuration.
// Host must be an HTTP or HTTPS base URL. Timeout, when set, must be a positive
// duration; TLS, when set, must pass its own validation.
func (m *Config) Validate() error {
	return validationconfig.ValidateStruct(m,
		validation.Field(&m.Host, validation.Required, validation.By(m.validateHost), is.URL),
		validation.Field(&m.Timeout, validationconfig.NonNegativeDuration()),
		validation.Field(&m.TLS, validation.NilOrNotEmpty),
	)
}

func (m *Config) validateHost(_ any) error {
	u, err := url.Parse(m.Host)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") ||
		u.RawQuery != "" || u.ForceQuery || strings.Contains(m.Host, "#") {
		return ErrInvalidHost
	}
	if m.TLS != nil && u.Scheme != "https" {
		return ErrTLSRequiresHTTPS
	}
	return nil
}
