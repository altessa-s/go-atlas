// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"cmp"
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/data/meilisearch"
	"github.com/altessa-s/go-atlas/observability/health"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	tlsfactory "github.com/altessa-s/go-atlas/security/tlsutils/factory"
)

// defaultHealthServiceName is used when registering a health checker if
// [ClientBuilder.UseHealthServiceName] has not been called.
const defaultHealthServiceName = "meilisearch"

// ErrConfigRequired is returned by [ClientBuilder.Build] when the
// builder was constructed with a nil config. Exported so callers that
// build the factory dynamically (e.g. from a YAML loader pipeline that
// may omit the Meilisearch block) can branch on the failure.
var ErrConfigRequired = errors.New("meilisearch factory: configuration is required")

// ClientBuilder assembles a [meilisearch.Client] step by step using a fluent
// API. Create instances with [New]. Errors are accumulated and reported at
// [ClientBuilder.Build] time. The builder is not safe for concurrent use.
type ClientBuilder struct {
	corefactory.Base
	cfg  *config.Meilisearch
	errs []error

	// Dependencies
	healthCoordinator *health.Coordinator
	healthServiceName string
}

// New creates a [ClientBuilder] for the given Meilisearch config.
// Config can be nil — the error surfaces at [ClientBuilder.Build] time
// as [ErrConfigRequired].
func New(cfg *config.Meilisearch) *ClientBuilder {
	return &ClientBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build creates a [meilisearch.Client] from configuration. The client
// performs an initial health check during construction and Build returns
// an error if the server is unreachable. ctx bounds the initial probe
// so a slow / hung server cannot block startup beyond the caller's
// deadline. If a [health.Coordinator] was provided via
// [ClientBuilder.UseHealthCoordinator], a health checker is registered
// under the service name configured by
// [ClientBuilder.UseHealthServiceName] (defaulting to "meilisearch").
func (b *ClientBuilder) Build(ctx context.Context) (*meilisearch.Client, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, ErrConfigRequired
	}

	opts := []meilisearch.Option{
		meilisearch.WithLogger(b.Logger()),
		meilisearch.WithTimeout(b.cfg.Timeout),
	}
	if key := b.cfg.APIKey.Expose(); key != "" {
		opts = append(opts, meilisearch.WithAPIKey(key))
	}

	if b.cfg.TLS != nil {
		httpClient, err := b.buildHTTPClient(b.cfg.TLS, b.cfg.Timeout)
		if err != nil {
			return nil, b.WrapError(err, "build TLS http.Client")
		}
		opts = append(opts, meilisearch.WithHTTPClient(httpClient))
	}

	client, err := meilisearch.New(ctx, b.cfg.Host, opts...)
	if err != nil {
		return nil, b.WrapError(err, "create meilisearch client")
	}

	if b.healthCoordinator != nil {
		b.healthCoordinator.RegisterService(
			cmp.Or(b.healthServiceName, defaultHealthServiceName),
			&meilisearchHealthChecker{client: client, logger: b.Logger()},
		)
	}

	return client, nil
}

// buildHTTPClient resolves the YAML TlsClient block into an *http.Client
// suitable for the Meilisearch SDK. Delegates the TLS-config construction
// to security/tlsutils/factory so the SkipVerifyMode safety guard, CA
// pool assembly, mTLS client-cert loading, and ATLAS_ALLOW_INSECURE_TLS
// env-var handling all happen in the single canonical place — the
// Meilisearch factory does not reimplement any of it.
//
// timeout is taken from the Meilisearch block and applied to the
// resulting http.Client so the WithTimeout option still bounds every
// outbound request, including the synchronous startup probe.
func (b *ClientBuilder) buildHTTPClient(tlsCfg *config.TlsClient, timeout time.Duration) (*http.Client, error) {
	tlsConfig, err := tlsfactory.New(nil).UseLogger(b.Logger()).CreateClientConfig(tlsCfg)
	if err != nil {
		return nil, err
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}, nil
}

// Compile-time assertion: tls.Config is referenced indirectly via
// http.Transport.TLSClientConfig — keep the import stable in case the
// build flips between Go versions that strip unused imports more
// aggressively. The actual value is constructed via tlsfactory.
var _ = (*tls.Config)(nil)
