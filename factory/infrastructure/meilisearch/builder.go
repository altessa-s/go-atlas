// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/altessa-s/go-atlas-core/types/nilcheck"
	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/store/meilisearch"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
	httpclient "github.com/altessa-s/go-atlas-net/http/client"
	meilisearchconfig "github.com/altessa-s/go-atlas/config/meilisearch"
	tlsconfig "github.com/altessa-s/go-atlas/config/tls"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	tlsfactory "github.com/altessa-s/go-atlas/factory/ext/tlsutils"
)

// defaultHealthServiceName is used when registering a health checker if
// [ClientBuilder.UseHealthServiceName] has not been called.
const defaultHealthServiceName = "meilisearch"

// ErrConfigRequired is returned by [ClientBuilder.Build] when the
// builder was constructed with a nil config. Exported so callers that
// build the factory dynamically (e.g. from a YAML loader pipeline that
// may omit the Meilisearch block) can branch on the failure.
var ErrConfigRequired = fmt.Errorf("meilisearch factory: %w", corefactory.ErrConfigRequired)

// ClientBuilder assembles a [meilisearch.Client] step by step using a fluent
// API. Create instances with [New]. Errors are accumulated and reported at
// [ClientBuilder.Build] time. The builder is not safe for concurrent use.
type ClientBuilder struct {
	corefactory.Base
	cfg  *meilisearchconfig.Config
	errs []error

	// Dependencies
	healthCoordinator health.Registrar
	healthServiceName string
	noHealth          bool
}

// New creates a [ClientBuilder] for the given Meilisearch config.
// Config can be nil — the error surfaces at [ClientBuilder.Build] time
// as [ErrConfigRequired].
func New(cfg *meilisearchconfig.Config) *ClientBuilder {
	return &ClientBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build creates a [meilisearch.Client] from configuration. The client
// performs an initial health check during construction and Build returns
// an error if the server is unavailable. ctx bounds the initial probe
// so a slow / hung server cannot block startup beyond the caller's
// deadline. If a [health.Registrar] was provided via
// [ClientBuilder.UseHealthCoordinator], a health checker is registered
// under the service name configured by
// [ClientBuilder.UseHealthServiceName] (defaulting to "meilisearch"),
// unless disabled with [ClientBuilder.WithoutHealth].
func (b *ClientBuilder) Build(ctx context.Context) (*meilisearch.Client, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, ErrConfigRequired
	}
	if err := b.cfg.Validate(); err != nil {
		return nil, b.WrapError(err, "validate meilisearch config")
	}

	timeout := b.cfg.Timeout
	if timeout <= 0 {
		timeout = meilisearch.DefaultTimeout
	}
	opts := []meilisearch.Option{
		meilisearch.WithLogger(b.Logger()),
		meilisearch.WithTimeout(timeout),
	}
	key := b.cfg.APIKey.Expose()
	opts = coreslices.AppendIf(opts, key != "", meilisearch.WithAPIKey(key))

	if b.cfg.TLS != nil {
		httpClient, err := b.buildHTTPClient(b.cfg.TLS, timeout)
		if err != nil {
			return nil, b.WrapError(err, "build TLS http.Client")
		}
		opts = append(opts, meilisearch.WithHTTPClient(httpClient))
	}

	client, err := meilisearch.New(ctx, b.cfg.Host, opts...)
	if err != nil {
		return nil, b.WrapError(err, "create meilisearch client")
	}

	if !nilcheck.IsNil(b.healthCoordinator) && !b.noHealth {
		b.healthCoordinator.RegisterService(
			cmp.Or(b.healthServiceName, defaultHealthServiceName),
			&meilisearchHealthChecker{client: client, logger: b.Logger()},
		)
	}

	return client, nil
}

// buildHTTPClient resolves the YAML TlsClient block into an *http.Client
// suitable for the Meilisearch SDK. Delegates the TLS-config construction
// to factory/ext/tlsutils so the SkipVerifyMode safety guard, CA
// pool assembly, mTLS client-cert loading, and ATLAS_ALLOW_INSECURE_TLS
// env-var handling all happen in the single canonical place — the
// Meilisearch factory does not reimplement any of it.
//
// timeout is taken from the Meilisearch block and applied to the
// resulting http.Client so the WithTimeout option still bounds every
// outbound request, including the synchronous startup probe.
func (b *ClientBuilder) buildHTTPClient(tlsCfg *tlsconfig.Client, timeout time.Duration) (*http.Client, error) {
	tlsConfig, err := tlsfactory.CreateClientConfig(tlsCfg, b.Logger())
	if err != nil {
		return nil, err
	}
	transport := httpclient.DefaultPooledTransport()
	transport.Proxy = nil
	transport.TLSClientConfig = tlsConfig
	return &http.Client{Timeout: cmp.Or(timeout, meilisearch.DefaultTimeout), Transport: transport}, nil
}
