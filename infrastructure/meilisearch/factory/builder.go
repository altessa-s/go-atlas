// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"cmp"
	"context"
	"errors"
	"log/slog"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/data/meilisearch"
	"github.com/altessa-s/go-atlas/observability/health"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
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
