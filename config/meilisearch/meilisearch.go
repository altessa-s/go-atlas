// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package meilisearchconfig

import (
	"time"

	"github.com/go-ozzo/ozzo-validation/v4/is"

	"github.com/altessa-s/go-atlas/core/types/redacted"

	tlsconfig "github.com/altessa-s/go-atlas/config/tls"
	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for Meilisearch configuration.
const (
	defaultMeilisearchHost    = "http://localhost:7700"
	defaultMeilisearchTimeout = 30 * time.Second
)

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
	// Host is the Meilisearch server URL.
	// Defaults to "http://localhost:7700".
	Host string `yaml:"host" default:"http://localhost:7700"`

	// APIKey is the master or admin API key sent in the Authorization
	// header. Leave empty to disable authentication — useful for local
	// development against an open instance.
	APIKey redacted.RedactedString `yaml:"apiKey"`

	// Timeout is the HTTP client timeout for Meilisearch requests.
	// Defaults to 30 seconds.
	Timeout time.Duration `yaml:"timeout" default:"30s"`

	// TLS configures the HTTPS transport. When non-nil, the factory
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
// Host must be a non-empty URL; Timeout, when set, must be a positive
// duration; TLS, when set, must pass its own validation.
func (m *Config) Validate() error {
	return validationconfig.ValidateStruct(m,
		validation.Field(&m.Host, validation.Required, is.URL),
		validation.Field(&m.Timeout, ozzo_rules.DurationOrZero()),
		validation.Field(&m.TLS, validation.NilOrNotEmpty),
	)
}
