// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package meilisearch

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
	coreio "github.com/altessa-s/go-atlas-core/io"
	httpclient "github.com/altessa-s/go-atlas-net/http/client"
	msdk "github.com/meilisearch/meilisearch-go"
)

// loggerComponent is added to every log line emitted by the client.
const loggerComponent = "meilisearch-client"

// Client wraps the meilisearch-go SDK with an HTTP client we own (so idle
// connections can be released at shutdown) and a structured logger.
//
// Create instances with [New], or build one from configuration via
// factory/infrastructure/meilisearch.
type Client struct {
	sdk        msdk.ServiceManager
	httpClient *http.Client
	logger     *slog.Logger
}

// New creates a [Client] targeting host and verifies connectivity with a
// Health round-trip. Returns an error if the server is unavailable.
//
// ctx bounds the initial health probe — pass a context with a deadline
// (or [context.Background] if startup should only be capped by the
// configured timeout). host is the Meilisearch base URL (e.g.
// http://localhost:7700). Pass [WithAPIKey], [WithTimeout], [WithLogger],
// or [WithHTTPClient] to override the defaults.
func New(ctx context.Context, host string, opts ...Option) (*Client, error) {
	o := newOptions(opts...)

	endpoint, err := url.Parse(host)
	if err != nil || endpoint.Hostname() == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") ||
		endpoint.RawQuery != "" || endpoint.ForceQuery || strings.Contains(host, "#") {
		return nil, ErrInvalidHost
	}

	httpClient := o.httpClient
	if httpClient == nil {
		transport := httpclient.DefaultPooledTransport()
		transport.Proxy = nil
		httpClient = &http.Client{Timeout: o.timeout, Transport: transport}
	}

	sdkOpts := []msdk.Option{msdk.WithCustomClient(wrapHTTPClient(httpClient)), msdk.DisableRetries()}
	sdkOpts = coreslices.AppendIf(sdkOpts, o.apiKey != "", msdk.WithAPIKey(o.apiKey))

	sdk := msdk.New(host, sdkOpts...)

	c := &Client{
		sdk:        sdk,
		httpClient: httpClient,
		logger:     o.logger.With(slog.String("component", loggerComponent)),
	}

	if err := c.Health(ctx); err != nil {
		httpClient.CloseIdleConnections()
		return nil, err
	}

	c.logger.InfoContext(ctx, "meilisearch client initialized", slog.String("host", endpoint.Host))

	return c, nil
}

// Health checks that the server reports "available".
// Failures match [ErrHealthCheck] and omit response and SDK error text.
func (c *Client) Health(ctx context.Context) error {
	status, err := c.sdk.Health(context.WithValue(ctx, healthRequestKey{}, true))
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return errors.Join(ErrHealthCheck, ctxErr)
		}
		for _, cause := range []error{coreio.ErrReadLimitExceeded, io.ErrUnexpectedEOF, context.DeadlineExceeded} {
			if errors.Is(err, cause) {
				return errors.Join(ErrHealthCheck, cause)
			}
		}
		return ErrHealthCheck
	}
	if status == nil || status.Status != "available" {
		return ErrHealthCheck
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
