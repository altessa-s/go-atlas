// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/altessa-s/go-atlas/core/time/timeformat"
	"github.com/altessa-s/go-atlas/transport/grpc/client/pool"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	corecontext "github.com/altessa-s/go-atlas/core/context"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	grpcinterceptors "github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	grpcerrstatus "github.com/altessa-s/go-atlas/transport/grpc/interceptors/errstatus"
	grpclogger "github.com/altessa-s/go-atlas/transport/grpc/interceptors/logger"
)

// Client is a generic gRPC client with connection management, retry, and health checking.
// It provides a foundation for building service-specific gRPC clients with consistent
// behavior for connection pooling, automatic retry, logging, and error handling.
//
// Create a Client with [New]. In single-connection mode (default), the client holds
// one persistent [grpc.ClientConn]. In pool mode (when [WithConnectionPool] is used),
// connections are obtained from a [pool.ConnectionPool] and must be returned via
// [Client.ReturnConnection].
//
// All exported methods are safe for concurrent use after construction. The
// underlying [grpc.ClientConn] and [pool.ConnectionPool] handle their own
// synchronization.
//
// Basic usage (single connection mode):
//
//	c, err := client.New(ctx, "localhost:8080",
//		client.WithInsecure(),
//	)
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer c.Close(ctx)
//
// With connection pool (recommended for high-throughput):
//
//	p := pool.New(
//		pool.WithPoolSize(20),
//		pool.WithMaxIdleTime(time.Hour),
//	)
//	stop, _ := p.Start(ctx)
//	defer stop()
//
//	c, err := client.New(ctx, "localhost:8080",
//		client.WithConnectionPool(p),
//		client.WithRetry(),
//	)
type Client struct {
	// Connection management
	conn *grpc.ClientConn     // Used when pool is nil (single connection mode)
	pool *pool.ConnectionPool // Used for connection pooling

	// Configuration
	appName string
	address string

	// TLS and logging
	tlsConfig *tls.Config
	logger    *slog.Logger

	// Retry configuration
	retryConfig *RetryConfig

	// Timeouts
	mutationTimeout time.Duration // Timeout for Create/Update/Delete operations
	queryTimeout    time.Duration // Timeout for Get/List operations

	// Additional gRPC options
	extraDialOptions []grpc.DialOption

	// Custom error converter
	errorConverter ErrorConverter

	insecure    bool
	enableRetry bool
}

// New creates a new gRPC client connected to address.
//
// In single-connection mode (the default), a persistent connection is established
// immediately. In pool mode (when [WithConnectionPool] is provided), no connection
// is created until [Client.GetConnection] is called.
//
// The returned client must be closed with [Client.Close] when no longer needed
// (pool-mode clients are closed through the pool's stop function instead).
func New(ctx context.Context, address string, opts ...Option) (*Client, error) {
	c := &Client{
		address:         address,
		logger:          slog.New(slog.DiscardHandler),
		tlsConfig:       defaultSecureTLSConfig(),
		mutationTimeout: DefaultMutationTimeout,
		queryTimeout:    DefaultQueryTimeout,
	}

	for _, opt := range opts {
		opt(c)
	}

	// Connect to the gRPC server
	if err := c.connect(ctx); err != nil {
		return nil, err
	}

	return c, nil
}

// connect establishes the connection to the gRPC server.
// If pool is configured, this method does nothing as connections are managed by the pool.
// Otherwise, it creates a single persistent connection.
func (c *Client) connect(ctx context.Context) error {
	// If using connection pool, skip creating a dedicated connection
	if c.pool != nil {
		c.logger.InfoContext(ctx, "using connection pool mode", "address", c.address)
		return nil
	}

	// Single connection mode: create a persistent connection
	dialOpts, err := c.dialOptions() //nolint:contextcheck // dialOptions builds static config, no context needed
	if err != nil {
		return coreerrs.WrapOperation(err, "build dial options")
	}
	conn, err := grpc.NewClient(c.address, dialOpts...)
	if err != nil {
		return coreerrs.Wrapf(err, "failed to connect to %s", c.address)
	}

	c.conn = conn
	c.logger.InfoContext(ctx, "created single connection", "address", c.address)

	return nil
}

// Close closes the underlying gRPC connection.
// In pool mode this is a no-op because the pool owns the connections.
// Close is idempotent and safe to call on a nil connection.
func (c *Client) Close(_ context.Context) error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// GetConnection returns a connection from the pool or the single connection.
// When using pool mode, the returned connection MUST be returned via ReturnConnection.
// When using single connection mode, calling ReturnConnection is a no-op.
func (c *Client) GetConnection(ctx context.Context) (*grpc.ClientConn, error) {
	if c.pool != nil {
		return c.pool.GetConnection(ctx, c.address)
	}
	return c.conn, nil
}

// ReturnConnection returns a connection to the pool.
// This is a no-op when using single connection mode.
func (c *Client) ReturnConnection(conn *grpc.ClientConn) {
	if c.pool != nil {
		c.pool.ReturnConnection(conn)
	}
}

// HealthCheck reports whether the gRPC connection is usable.
// A connection in the [connectivity.Ready] or [connectivity.Idle] state is
// considered healthy. All other states produce an error describing the state.
func (c *Client) HealthCheck(ctx context.Context) error {
	conn, err := c.GetConnection(ctx)
	if err != nil {
		return coreerrs.WrapOperation(err, "get connection")
	}
	defer c.ReturnConnection(conn)

	state := conn.GetState()
	c.logger.DebugContext(ctx, "connection health check", "state", state.String(), "address", c.address)

	// Ready and Idle are considered healthy states
	switch state {
	case connectivity.Ready, connectivity.Idle:
		return nil
	default:
		return fmt.Errorf("unhealthy connection state: %s", state.String())
	}
}

// Address returns the server address.
func (c *Client) Address() string {
	return c.address
}

// Logger returns the configured logger.
func (c *Client) Logger() *slog.Logger {
	return c.logger
}

// MutationTimeout returns the configured timeout for mutation operations.
func (c *Client) MutationTimeout() time.Duration {
	return c.mutationTimeout
}

// QueryTimeout returns the configured timeout for query operations.
func (c *Client) QueryTimeout() time.Duration {
	return c.queryTimeout
}

// ApplyTimeout derives a context with the given timeout unless one is already set.
// When timeout is zero or the context already carries a deadline, the original context
// is returned. The caller must always call the returned cancel function.
func (c *Client) ApplyTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return corecontext.ApplyTimeout(ctx, timeout)
}

// ApplyMutationTimeout applies the mutation timeout to the context.
func (c *Client) ApplyMutationTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return corecontext.ApplyTimeout(ctx, c.mutationTimeout)
}

// ApplyQueryTimeout applies the query timeout to the context.
func (c *Client) ApplyQueryTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return corecontext.ApplyTimeout(ctx, c.queryTimeout)
}

// healthCheckConfig is the default gRPC service configuration with health checking enabled.
var healthCheckConfig = `{
  "healthCheckConfig": {
    "serviceName": "health.v1.HealthService"
  }
}`

// retryConfigTemplate is the gRPC service configuration template with health checking,
// round-robin load balancing, and retry policy.
var retryConfigTemplate = `{
  "healthCheckConfig": {
    "serviceName": "health.v1.HealthService"
  },
  "loadBalancingConfig": [ { "round_robin": {} } ],
  "methodConfig": [{
    "waitForReady": false,
    "name": [{}],
    "timeout": "%s",
    "retryPolicy": {
      "maxAttempts": %d,
      "initialBackoff": "%s",
      "maxBackoff": "%s",
      "backoffMultiplier": %f,
      "retryableStatusCodes": [%s]
    }
  }]
}`

// buildServiceConfig builds gRPC service configuration with health checking and optional retry.
func (c *Client) buildServiceConfig() string {
	if !c.enableRetry || c.retryConfig == nil {
		return healthCheckConfig
	}

	cfg := c.retryConfig
	return fmt.Sprintf(retryConfigTemplate,
		(time.Duration(cfg.MaxAttempts) * cfg.MaxBackoff).String(),
		cfg.MaxAttempts,
		cfg.InitialBackoff.String(),
		cfg.MaxBackoff.String(),
		cfg.BackoffMultiplier,
		joinStatusCodes(cfg.RetryableStatusCodes),
	)
}

// joinStatusCodes formats status codes as a JSON array string with quoted elements.
func joinStatusCodes(codes []string) string {
	if len(codes) == 0 {
		return `"UNAVAILABLE"`
	}
	quoted := slices.Collect(coreslices.Map(codes, func(code string) string {
		return `"` + code + `"`
	}))
	return strings.Join(quoted, ", ")
}

// dialOptions returns the gRPC dial options for the client connection.
func (c *Client) dialOptions() ([]grpc.DialOption, error) {
	serviceConfig := c.buildServiceConfig()

	grpcOptions := []grpc.DialOption{
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff:           backoff.DefaultConfig,
			MinConnectTimeout: MinConnectTimeout,
		}),
		grpc.WithDefaultServiceConfig(serviceConfig),
		//nolint:mnd
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                KeepaliveTime,
			Timeout:             KeepaliveTimeout,
			PermitWithoutStream: false,
		}),
	}

	// Add interceptors chain
	chain := grpcinterceptors.NewChain(
		grpclogger.ClientInterceptor(
			grpclogger.Slog(c.logger),
			grpclogger.WithTimeFormat(timeformat.RFC3339),
		),
		grpcerrstatus.ClientInterceptor(
			grpcerrstatus.WithStatusConverterFunc(c.convertError),
		),
	)

	clientOptions, err := chain.ClientOptions()
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "build interceptor chain")
	}
	grpcOptions = append(grpcOptions, clientOptions...)

	// Add transport credentials
	if c.insecure {
		grpcOptions = append(grpcOptions, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		grpcOptions = append(grpcOptions, grpc.WithTransportCredentials(credentials.NewTLS(c.tlsConfig)))
	}

	// Add user agent
	if c.appName != "" {
		grpcOptions = append(grpcOptions, grpc.WithUserAgent(c.appName))
	}

	// Add custom dial options
	grpcOptions = append(grpcOptions, c.extraDialOptions...)

	return grpcOptions, nil
}

// convertError converts a gRPC status to an error using the configured error converter.
func (c *Client) convertError(ctx context.Context, st *status.Status) error {
	if c.errorConverter != nil {
		return c.errorConverter(ctx, st)
	}
	return ParseStatusError(st)
}

// defaultSecureTLSConfig returns a TLS configuration with secure defaults.
// Enforces TLS 1.2 as minimum version to prevent downgrade attacks.
func defaultSecureTLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
}
