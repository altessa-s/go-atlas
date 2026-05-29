// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"time"

	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for the YAML-facing OCSP stapling configuration.
//
// The constants intentionally mirror the runtime defaults in
// security/tlsutils/ocsp so an operator who copies the template
// gets the same behavior as the Go API caller who passes no options.
const (
	// DefaultTlsProviderOCSPFailureMode is the YAML-facing default for
	// [TlsProviderOCSP.FailureMode]: production-safe Soft (availability
	// over strict revocation). Operators MUST explicitly opt in to
	// "hard" — hard-fail breaks fresh connections whenever the OCSP
	// responder is down.
	DefaultTlsProviderOCSPFailureMode = "soft"

	// DefaultTlsProviderOCSPEnableCompression matches the runtime
	// default: response compression on. Cache entries are typically
	// 1–2 KiB; gzip cuts that by 50–70 % at negligible CPU cost.
	DefaultTlsProviderOCSPEnableCompression = true

	// DefaultTlsProviderOCSPMaxCacheEntries mirrors
	// ocsp.DefaultMaxCacheEntries and bounds the in-memory response
	// cache. Realistic workloads (a few hundred client certs in mTLS,
	// or a single server staple) fit comfortably; 4096 is large enough
	// to absorb spikes without letting an mTLS server with many
	// distinct client certs grow the cache unbounded between cleanup
	// ticks.
	DefaultTlsProviderOCSPMaxCacheEntries = 4096

	// DefaultTlsProviderOCSPHTTPTimeout caps a single OCSP responder
	// fetch. Hard-fail mode multiplies this through every fresh
	// connection that misses the cache, so keep it tight.
	DefaultTlsProviderOCSPHTTPTimeout = 10 * time.Second
)

// TlsProviderOCSP configures the OCSP stapler used by the TLS providers
// registered through [security/tlsutils/factory.ProvidersBuilder]. When
// Enabled is true and no stapler was injected programmatically via
// UseOcspStapler, the builder constructs a stapler from these fields and
// attaches it to every file / Vault / S3 provider it builds.
//
// Letting OCSP live alongside the certificate provider config (instead
// of in a sibling block) keeps the YAML surface small: the operator
// turning on OCSP for "their TLS" doesn't need to track a second
// section in lock-step with the certificate source.
type TlsProviderOCSP struct {
	// Enabled toggles automatic OCSP stapler construction. When false
	// the builder does not create a stapler — providers still load
	// certificates, just without an OCSP staple attached.
	Enabled bool `yaml:"enabled" default:"false"`

	// FailureMode controls how the stapler reacts when it cannot fetch
	// a valid OCSP response for a certificate. Supported values:
	//
	//   - "soft" (default): return the cert without an OCSP staple
	//     when the responder is unreachable or returns an invalid
	//     response. Maximizes availability — a transient OCSP outage
	//     does not break new connections — but a revoked certificate
	//     may be served alongside its valid TLS material.
	//   - "hard": return an error from the GetCertificate /
	//     GetClientCertificate callbacks when an OCSP staple cannot
	//     be produced, aborting the TLS handshake. Use this in
	//     environments where the cost of a temporary outage is
	//     acceptable but the cost of serving a possibly-revoked
	//     certificate is not (PCI-DSS, regulated industries,
	//     security-sensitive APIs).
	FailureMode string `yaml:"failureMode" default:"soft"`

	// EnableCompression turns on gzip compression of cached OCSP
	// responses. Cache entries are typically 1–2 KiB; compression
	// cuts that by 50–70 % at negligible CPU cost. Defaults to true.
	EnableCompression bool `yaml:"enableCompression" default:"true"`

	// MaxCacheEntries bounds the in-memory OCSP response cache.
	// Set to 0 to disable the cap (legacy unbounded behavior — only
	// safe when the certificate population is small and known).
	// Defaults to 4096.
	MaxCacheEntries int `yaml:"maxCacheEntries" default:"4096"`

	// RefreshSchedule is the cron expression for periodic OCSP
	// refresh. When set AND a scheduler dependency is injected via
	// the providers builder, RunRefreshAll is registered as a
	// recurring task — entries are refreshed before they expire so
	// fresh TLS handshakes never block on an OCSP fetch.
	//
	// Empty (the default) leaves refresh to lazy on-demand fetches
	// inside GetOCSPStaple. That is fine for low-traffic services
	// but allocates an OCSP HTTP request on the critical path of
	// every cache-miss handshake.
	//
	// Format: 6-field cron (seconds-precision) — same as the rest of
	// the project's scheduler tasks.
	RefreshSchedule string `yaml:"refreshSchedule"`

	// HTTPTimeout caps a single OCSP responder fetch. Hard-fail mode
	// multiplies this through every fresh connection that misses the
	// cache, so keep it tight. Defaults to 10s.
	HTTPTimeout time.Duration `yaml:"httpTimeout" default:"10s"`
}

// DefaultTlsProviderOCSP returns an OCSP configuration with default
// values. Mirrors the constructor pattern used by the rest of the
// config package.
func DefaultTlsProviderOCSP() TlsProviderOCSP {
	return TlsProviderOCSP{
		Enabled:           false,
		FailureMode:       DefaultTlsProviderOCSPFailureMode,
		EnableCompression: DefaultTlsProviderOCSPEnableCompression,
		MaxCacheEntries:   DefaultTlsProviderOCSPMaxCacheEntries,
		HTTPTimeout:       DefaultTlsProviderOCSPHTTPTimeout,
	}
}

// Validate performs validation on the OCSP configuration. Validation
// runs unconditionally — even when Enabled is false — so a half-typed
// failureMode in a disabled block still surfaces at config-load time
// rather than at the moment the operator flips Enabled to true.
func (c *TlsProviderOCSP) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.FailureMode,
			ozzo_rules.OneOf("soft", "hard"),
		),
		validation.Field(&c.MaxCacheEntries, validation.Min(0)),
		validation.Field(&c.HTTPTimeout, ozzo_rules.DurationOrZero()),
	)
}
