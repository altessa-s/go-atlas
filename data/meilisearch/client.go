// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package meilisearch

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

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
// host is the Meilisearch base URL (e.g. http://localhost:7700). Pass
// [WithAPIKey], [WithTimeout], or [WithLogger] to override the defaults.
func New(host string, opts ...Option) (*Client, error) {
	o := newOptions(opts)

	httpClient := &http.Client{Timeout: o.timeout}

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

	if _, err := sdk.Health(); err != nil {
		return nil, fmt.Errorf("meilisearch health check failed: %w", err)
	}

	c.logger.Info("meilisearch client initialized", slog.String("host", host))

	return c, nil
}

// Health pings the server and returns an error if it is unreachable.
// Honours ctx for cancellation.
func (c *Client) Health(ctx context.Context) error {
	if _, err := c.sdk.HealthWithContext(ctx); err != nil {
		return fmt.Errorf("meilisearch health check failed: %w", err)
	}
	return nil
}

// Close releases idle HTTP connections held by the underlying client.
func (c *Client) Close() error {
	c.httpClient.CloseIdleConnections()
	c.logger.Debug("meilisearch client closed")

	return nil
}
