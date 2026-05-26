// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package meilisearch

import (
	"context"
	"log/slog"
	"net/http"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	msdk "github.com/meilisearch/meilisearch-go"
)

// loggerComponent is added to every log line emitted by the client.
const loggerComponent = "meilisearch-client"

// Client wraps the meilisearch-go SDK with an HTTP client we own (so idle
// connections can be released at shutdown) and a structured logger.
//
// Create instances with [New], or build one from configuration via
// infrastructure/meilisearch/factory.
type Client struct {
	sdk        msdk.ServiceManager
	httpClient *http.Client
	logger     *slog.Logger
}

// New creates a [Client] targeting host and verifies connectivity with a
// Health round-trip. Returns an error if the server is unreachable.
//
// ctx bounds the initial health probe — pass a context with a deadline
// (or [context.Background] if startup should only be capped by the
// configured timeout). host is the Meilisearch base URL (e.g.
// http://localhost:7700). Pass [WithAPIKey], [WithTimeout], [WithLogger],
// or [WithHTTPClient] to override the defaults.
func New(ctx context.Context, host string, opts ...Option) (*Client, error) {
	o := newOptions(opts...)

	httpClient := o.httpClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: o.timeout}
	}

	sdkOpts := []msdk.Option{msdk.WithCustomClient(httpClient)}
	if o.apiKey != "" {
		sdkOpts = append(sdkOpts, msdk.WithAPIKey(o.apiKey))
	}

	sdk := msdk.New(host, sdkOpts...)

	c := &Client{
		sdk:        sdk,
		httpClient: httpClient,
		logger:     o.logger.With(slog.String("component", loggerComponent)),
	}

	if _, err := sdk.HealthWithContext(ctx); err != nil {
		return nil, coreerrs.WrapOperation(err, "meilisearch health check")
	}

	c.logger.InfoContext(ctx, "meilisearch client initialized", slog.String("host", host))

	return c, nil
}

// Health pings the server and returns an error if it is unreachable.
// Honors ctx for cancellation.
func (c *Client) Health(ctx context.Context) error {
	if _, err := c.sdk.HealthWithContext(ctx); err != nil {
		return coreerrs.WrapOperation(err, "meilisearch health check")
	}
	return nil
}

// Close releases idle HTTP connections held by the underlying client.
// Always returns nil — kept as an io.Closer-compatible signature so
// callers can defer it uniformly.
func (c *Client) Close() error {
	c.httpClient.CloseIdleConnections()
	c.logger.Debug("meilisearch client closed")

	return nil
}
