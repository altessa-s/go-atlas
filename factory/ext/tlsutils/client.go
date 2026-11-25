// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"crypto/tls"
	"errors"
	"log/slog"

	"github.com/altessa-s/go-tlsutils"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	tlsconfig "github.com/altessa-s/go-atlas/config/tls"
)

// ErrInsecureSkipVerifyRejected is returned by [CreateClientConfig]
// when SkipVerify is true and the configured [tlsconfig.SkipVerifyMode] is
// [tlsconfig.SkipVerifyModeEnforce] (the default). Operators who genuinely
// need to skip verification must explicitly opt out via SkipVerifyMode=warn
// (logged) or SkipVerifyMode=disabled (silent, tests only).
var ErrInsecureSkipVerifyRejected = errors.New(
	"security/tlsutils: SkipVerify is true but SkipVerifyMode is enforce — refusing to disable certificate verification",
)

// CreateClientConfig creates a TLS configuration for client connections from config.
// Returns nil, nil if the configuration is nil.
// A nil logger discards warnings.
func CreateClientConfig(cfg *tlsconfig.Client, logger *slog.Logger) (*tls.Config, error) {
	if cfg == nil {
		return nil, nil //nolint:nilnil
	}

	caPool, err := tlsutils.BuildCAPool(true, cfg.CACerts...)
	if err != nil {
		return nil, coreerrs.Wrap(err, "failed to build CA pool")
	}

	var certs []tls.Certificate
	if cfg.Certificate != "" && cfg.PrivateKey != "" {
		cert, err := tlsutils.LoadFromFile(cfg.PrivateKey, cfg.Certificate, cfg.PrivateKeyPassword.Expose())
		if err != nil {
			return nil, coreerrs.Wrap(err, "failed to load client certificate")
		}
		certs = []tls.Certificate{*cert}
	}

	tlsConfig := tlsutils.DefaultClientTLSConfig(cfg.ServerName)
	if cfg.SkipVerify {
		if logger == nil {
			logger = slog.New(slog.DiscardHandler)
		}
		switch cfg.SkipVerifyMode {
		case tlsconfig.SkipVerifyModeEnforce:
			return nil, coreerrs.Wrap(ErrInsecureSkipVerifyRejected,
				"refusing to build TLS client config for server "+cfg.ServerName)
		case tlsconfig.SkipVerifyModeWarn:
			logger.Warn("TLS certificate verification is disabled — connections are susceptible to man-in-the-middle attacks",
				slog.String("server_name", cfg.ServerName))
			tlsConfig.InsecureSkipVerify = true
		case tlsconfig.SkipVerifyModeDisabled:
			// Explicit operator/test opt-out. Still log it: an operator who
			// disables verification should see runtime evidence of it, matching
			// the Warn mode above. Use only in tests with localhost fixtures.
			logger.Warn("TLS certificate verification disabled via TLSSkipVerifyModeDisabled (explicit opt-out) — "+
				"connections are susceptible to man-in-the-middle attacks",
				slog.String("server_name", cfg.ServerName))
			tlsConfig.InsecureSkipVerify = true
		default:
			// Empty or unrecognized mode — fail safe with the Enforce
			// error. The YAML loader fills in the default:"enforce"
			// struct tag, so this path is reached only by programmatic
			// callers that built TlsClient by hand and forgot to set
			// SkipVerifyMode, or by a typo in YAML. Either way we want
			// the operator to commit to a value explicitly rather than
			// inheriting a silent fallback.
			return nil, coreerrs.Wrap(ErrInsecureSkipVerifyRejected,
				"SkipVerifyMode must be one of enforce/warn/disabled (got "+string(cfg.SkipVerifyMode)+")")
		}
	}
	tlsConfig.RootCAs = caPool
	if len(certs) > 0 {
		tlsConfig.Certificates = certs
	}

	return tlsConfig, nil
}
